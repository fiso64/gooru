import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Job } from '$lib/api/types';

const { untrackSpy } = vi.hoisted(() => ({
  untrackSpy: vi.fn((value: unknown) => typeof value === 'function' ? (value as () => unknown)() : value)
}));

vi.mock('svelte', async () => {
  const actual = await vi.importActual<typeof import('svelte')>('svelte');
  return { ...actual, untrack: untrackSpy };
});

import { createUploadWorkflow } from './uploadWorkflow.svelte';

function uploadFile(name: string): File {
  return { name, size: 10, type: 'image/jpeg', lastModified: 0 } as File;
}

describe('createUploadWorkflow', () => {
  beforeEach(() => untrackSpy.mockClear());

  it('appends later file selections to the staged batch', () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg')]);
    workflow.select([uploadFile('second.jpg')]);

    expect(workflow.files.map((file) => file.name)).toEqual(['first.jpg', 'second.jpg']);
    expect(workflow.items.map((item) => [item.name, item.status])).toEqual([
      ['first.jpg', 'staged'],
      ['second.jpg', 'staged']
    ]);
  });

  it('drives displayed progress from transport callbacks instead of fabricated percentages', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);

    let release!: (value: Job) => void;
    const response = new Promise<Job>((resolve) => (release = resolve));
    const submitted = workflow.submit(async (variables) => {
      expect(workflow.items.map((item) => item.progress)).toEqual([0, 0]);
      variables.onProgress?.(37);
      expect(workflow.items.map((item) => item.progress)).toEqual([37, 37]);
      expect(workflow.status).toBe('Uploading 37%');
      return response;
    });

    release({
      id: 'job-progress',
      type: 'upload_import',
      status: 'pending',
      submitted_at: '2026-09-01T00:00:00Z'
    } as Job);
    await submitted;

    expect(workflow.items.map((item) => item.progress)).toEqual([100, 100]);
    expect(workflow.status).toBe('Upload complete · queued for import');
  });

  it('does not mark an interrupted transfer as 100% complete', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('large.mp4')]);

    await workflow.submit(async (variables) => {
      variables.onProgress?.(43);
      throw new Error('Network error while uploading files');
    });

    expect(workflow.items[0]).toMatchObject({ status: 'error', progress: 43 });
    expect(workflow.status).toContain('Network error');
  });

  it('does not let a new drop overwrite an active queued batch', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('queued.jpg')]);
    await workflow.submit(async () => ({
      id: 'job-queued',
      type: 'upload_import',
      status: 'pending',
      submitted_at: '2026-09-01T00:00:00Z'
    } as Job));

    workflow.select([uploadFile('later.jpg')]);
    expect(workflow.files.map((file) => file.name)).toEqual(['queued.jpg']);
    expect(workflow.items.map((item) => item.name)).toEqual(['queued.jpg']);
    expect(workflow.status).toContain('Upload in progress');
  });

  it('applies polled jobs outside the caller reactive dependency graph', () => {
    const workflow = createUploadWorkflow();
    const job = {
      id: 'job-1',
      type: 'upload_import',
      status: 'running',
      submitted_at: '2026-09-01T00:00:00Z',
      progress: 0.5
    } as Job;

    expect(workflow.applyJob(job)).toEqual({ completed: false, changedFiles: false });
    expect(untrackSpy).toHaveBeenCalledWith(expect.any(Function));
    expect(workflow.status).toBe('Importing');
  });
});
