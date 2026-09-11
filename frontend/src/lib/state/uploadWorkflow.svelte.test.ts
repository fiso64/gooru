import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Job } from '$lib/api/types';
import type { BackgroundOperation } from '$lib/api/operations';

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

function pendingJob(id: string, completed = 0, total = 2): Job & BackgroundOperation {
  return {
    id,
    type: 'upload_import',
    kind: 'upload_import',
    status: completed > 0 ? 'running' : 'pending',
    progress: total > 0 ? completed / total : 0,
    progress_total: total,
    progress_completed: completed,
    progress_failed: 0,
    submitted_at: '2026-09-11T00:00:00Z',
    created_at: '2026-09-11T00:00:00Z'
  } as Job & BackgroundOperation;
}

describe('createUploadWorkflow aggregate uploads', () => {
  beforeEach(() => untrackSpy.mockClear());

  it('defaults browser uploads to rename conflicts', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg')]);
    let policy = '';
    await workflow.submit(async (variables) => {
      policy = variables.conflictPolicy;
      return pendingJob('job-batch', 0, 1);
    });
    expect(policy).toBe('rename');
  });

  it('appends later file selections to one staged batch', () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg')]);
    workflow.select([uploadFile('second.jpg')]);
    expect(workflow.files.map((file) => file.name)).toEqual(['first.jpg', 'second.jpg']);
  });

  it('submits one durable operation with per-file queue metadata arrays', async () => {
    const now = vi.spyOn(Date, 'now');
    now.mockReturnValueOnce(1_700_000_000_000).mockReturnValueOnce(1_700_000_010_000).mockReturnValue(1_800_000_000_000);
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg')]);
    workflow.select([uploadFile('second.jpg')]);
    workflow.setTarget('archive', 'reverse_queue');
    let calls = 0;

    await workflow.submit(async (variables) => {
      calls += 1;
      expect(variables.files.map((file) => file.name)).toEqual(['first.jpg', 'second.jpg']);
      expect(variables.queueTimeMs).toEqual([1_700_000_000_000, 1_700_000_010_000]);
      expect(variables.queueFirstTimeMs).toBe(1_700_000_000_000);
      expect(variables.queueLastTimeMs).toBe(1_700_000_010_000);
      expect(variables.queueIndex).toEqual([0, 1]);
      expect(variables.queueTotal).toEqual([2, 2]);
      expect(variables.addedAtStrategy).toBe('reverse_queue');
      variables.onProgress?.(37);
      expect(workflow.items.map((item) => [item.status, item.progress])).toEqual([
        ['uploading', 37],
        ['uploading', 37]
      ]);
      return pendingJob('job-batch');
    });

    expect(calls).toBe(1);
    expect(workflow.activeJobIDs).toEqual(['job-batch']);
    expect(workflow.items.map((item) => [item.status, item.progress])).toEqual([
      ['queued', 100],
      ['queued', 100]
    ]);
    now.mockRestore();
  });

  it('applies aggregate running progress to every file in the batch', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    await workflow.submit(async () => pendingJob('job-batch'));

    expect(workflow.applyJob(pendingJob('job-batch', 1, 2))).toEqual({ completed: false, changedFiles: false });
    expect(workflow.items.map((item) => [item.status, item.progress])).toEqual([
      ['importing', 50],
      ['importing', 50]
    ]);
    expect(workflow.activeJobID).toBe('job-batch');
  });

  it('maps one completed aggregate result back to all staged items', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    await workflow.submit(async () => pendingJob('job-batch'));
    const completed = {
      ...pendingJob('job-batch', 2, 2),
      status: 'completed',
      progress: 1,
      result: {
        files: [
          { name: 'first.jpg', size: 10, target_id: 'default', status: 'imported' },
          { name: 'second.jpg', size: 10, target_id: 'default', status: 'duplicate_existing' }
        ]
      }
    } as Job;

    expect(workflow.applyJob(completed)).toEqual({ completed: true, changedFiles: true });
    expect(workflow.activeJobIDs).toEqual([]);
    expect(workflow.items.map((item) => item.status)).toEqual(['imported', 'duplicate_existing']);
  });

  it('marks the whole selection failed when the aggregate transport fails', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    await workflow.submit(async (variables) => {
      variables.onProgress?.(43);
      throw new Error('Network error while uploading files');
    });
    expect(workflow.items.map((item) => [item.status, item.progress])).toEqual([
      ['error', 43],
      ['error', 43]
    ]);
    expect(workflow.items.every((item) => item.error?.includes('Network error'))).toBe(true);
  });

  it('cancels the single durable operation for the whole batch', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    await workflow.submit(async () => pendingJob('job-batch'));
    const canceled: string[] = [];
    const result = await workflow.cancel(async (id) => {
      canceled.push(id);
      return { ...pendingJob(id), status: 'canceled' } as Job;
    });
    expect(result).toEqual({ changed: true });
    expect(canceled).toEqual(['job-batch']);
    expect(workflow.items.map((item) => item.status)).toEqual(['canceled', 'canceled']);
  });

  it('does not let a new drop overwrite an active aggregate batch', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('queued.jpg')]);
    await workflow.submit(async () => pendingJob('job-batch', 0, 1));
    workflow.select([uploadFile('later.jpg')]);
    expect(workflow.files.map((file) => file.name)).toEqual(['queued.jpg']);
    expect(workflow.status).toContain('Upload in progress');
  });

  it('replaces managed target defaults while preserving user tags', () => {
    const workflow = createUploadWorkflow();
    workflow.setTarget('one', 'queue', ['project:inbox', 'source:upload']);
    expect(workflow.tags).toBe('project:inbox source:upload');
    workflow.tags += ' user:kept';
    workflow.setTarget('two', 'queue', ['source:upload', 'user:kept']);
    expect(workflow.tags).toBe('user:kept source:upload');
  });

  it('does not duplicate target defaults on repeated selection', () => {
    const workflow = createUploadWorkflow();
    workflow.setTarget('one', 'queue', ['project:inbox', 'project:inbox']);
    workflow.setTarget('one', 'queue', ['project:inbox']);
    expect(workflow.tags).toBe('project:inbox');
  });

  it('reapplies target defaults exactly for an explicit selection', () => {
    const workflow = createUploadWorkflow();
    workflow.setTarget('one', 'queue', ['project:inbox']);
    workflow.tags = 'project:inbox user:custom';
    workflow.setTarget('one', 'queue', ['project:inbox', 'source:upload'], true);
    expect(workflow.tags).toBe('project:inbox source:upload');
  });

  it('keeps user tags when an explicitly selected target has no defaults', () => {
    const workflow = createUploadWorkflow();
    workflow.setTarget('one', 'queue', ['project:inbox']);
    workflow.tags = 'project:inbox user:custom';
    workflow.setTarget('plain', 'queue', [], true);
    expect(workflow.tags).toBe('user:custom');
  });

  it('preserves managed target defaults and user tags after direct completion', async () => {
    const workflow = createUploadWorkflow();
    workflow.setTarget('one', 'queue', ['project:inbox']);
    workflow.tags = 'project:inbox user:custom';
    workflow.select([uploadFile('first.jpg')]);
    await workflow.submit(async () => ({
      files: [{ name: 'first.jpg', size: 10, target_id: 'one', status: 'uploaded' }]
    } as never));
    expect(workflow.files).toEqual([]);
    expect(workflow.tags).toBe('project:inbox user:custom');
  });
});
