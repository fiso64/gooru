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

function pendingJob(id: string): Job {
  return {
    id,
    type: 'upload_import',
    status: 'pending',
    submitted_at: '2026-09-01T00:00:00Z'
  } as Job;
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

  it('uploads each file independently with real per-file progress', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    const calls: string[] = [];

    await workflow.submit(async (variables) => {
      const name = variables.files[0].name;
      calls.push(name);
      expect(variables.files).toHaveLength(1);
      if (name === 'first.jpg') {
        variables.onProgress?.(37);
        expect(workflow.items.map((item) => item.progress)).toEqual([37, 0]);
        return pendingJob('job-first');
      }
      variables.onProgress?.(64);
      expect(workflow.items.map((item) => item.progress)).toEqual([100, 64]);
      return pendingJob('job-second');
    });

    expect(calls).toEqual(['first.jpg', 'second.jpg']);
    expect(workflow.items.map((item) => [item.status, item.progress])).toEqual([
      ['queued', 100],
      ['queued', 100]
    ]);
    expect(workflow.activeJobIDs).toEqual(['job-first', 'job-second']);
  });

  it('continues with sibling files after a transport failure', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('small-a.jpg'), uploadFile('huge.mp4'), uploadFile('small-b.jpg')]);

    await workflow.submit(async (variables) => {
      const name = variables.files[0].name;
      if (name === 'huge.mp4') {
        variables.onProgress?.(43);
        throw new Error('Network error while uploading files');
      }
      variables.onProgress?.(100);
      return pendingJob(`job-${name}`);
    });

    expect(workflow.items.map((item) => [item.name, item.status, item.progress])).toEqual([
      ['small-a.jpg', 'queued', 100],
      ['huge.mp4', 'error', 43],
      ['small-b.jpg', 'queued', 100]
    ]);
    expect(workflow.items[1].error).toContain('Network error');
    expect(workflow.activeJobIDs).toEqual(['job-small-a.jpg', 'job-small-b.jpg']);
  });

  it('does not let a new drop overwrite an active queued batch', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('queued.jpg')]);
    await workflow.submit(async () => pendingJob('job-queued'));

    workflow.select([uploadFile('later.jpg')]);
    expect(workflow.files.map((file) => file.name)).toEqual(['queued.jpg']);
    expect(workflow.items.map((item) => item.name)).toEqual(['queued.jpg']);
    expect(workflow.status).toContain('Upload in progress');
  });

  it('applies each tracked import job independently', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    await workflow.submit(async (variables) => pendingJob(`job-${variables.files[0].name}`));

    const running = {
      ...pendingJob('job-first.jpg'),
      status: 'running',
      progress: 0.5
    } as Job;

    expect(workflow.applyJob(running)).toEqual({ completed: false, changedFiles: false });
    expect(untrackSpy).toHaveBeenCalledWith(expect.any(Function));
    expect(workflow.items.map((item) => [item.status, item.progress])).toEqual([
      ['importing', 50],
      ['queued', 100]
    ]);
    expect(workflow.activeJobID).toBe('job-first.jpg');
  });

  it('advances polling to the next job after one completes', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    await workflow.submit(async (variables) => pendingJob(`job-${variables.files[0].name}`));

    const completed = {
      ...pendingJob('job-first.jpg'),
      status: 'completed',
      result: {
        files: [{ name: 'first.jpg', size: 10, target_id: '', status: 'imported' }]
      }
    } as Job;

    expect(workflow.applyJob(completed)).toEqual({ completed: true, changedFiles: true });
    expect(workflow.activeJobID).toBe('job-second.jpg');
    expect(workflow.items[0].status).toBe('imported');
    expect(workflow.items[1].status).toBe('queued');
  });

  it('cancels all per-file jobs from the existing batch cancel action', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    await workflow.submit(async (variables) => pendingJob(`job-${variables.files[0].name}`));
    const canceled: string[] = [];

    const result = await workflow.cancel(async (id) => {
      canceled.push(id);
      return { ...pendingJob(id), status: 'canceled' } as Job;
    });

    expect(result).toEqual({ changed: true });
    expect(canceled).toEqual(['job-first.jpg', 'job-second.jpg']);
    expect(workflow.activeJobIDs).toEqual([]);
    expect(workflow.items.map((item) => item.status)).toEqual(['canceled', 'canceled']);
  });
});
