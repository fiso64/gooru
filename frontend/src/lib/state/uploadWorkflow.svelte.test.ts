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

import { createUploadWorkflow, perFileUploadProgress } from './uploadWorkflow.svelte';

function uploadFile(name: string, size = 10): File {
  return { name, size, type: 'image/jpeg', lastModified: 0 } as File;
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
    progress_completed_prefix: completed,
    progress_failed: 0,
    submitted_at: '2026-09-11T00:00:00Z',
    created_at: '2026-09-11T00:00:00Z'
  } as Job & BackgroundOperation;
}

function completedJob(id: string, name: string): Job {
  return {
    ...pendingJob(id, 1, 1),
    status: 'completed',
    progress: 1,
    result: {
      files: [{ name, size: 10, target_id: 'default', status: 'imported' }]
    }
  } as Job;
}

describe('createUploadWorkflow aggregate uploads', () => {
  beforeEach(() => untrackSpy.mockClear());

  it('projects aggregate multipart progress across files in request order', () => {
    const files = [uploadFile('first.jpg', 10), uploadFile('second.jpg', 30)];
    expect(perFileUploadProgress(files, 25)).toEqual([100, 0]);
    expect(perFileUploadProgress(files, 50)).toEqual([100, 33]);
    expect(perFileUploadProgress(files, 100)).toEqual([100, 100]);
  });

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


  it('applies add, remove, and empty set operations across staged items only', async () => {
    const workflow = createUploadWorkflow();
    workflow.tags = 'shared initial';
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    await workflow.submit(async () => pendingJob('job-existing', 0, 2));
    workflow.select([uploadFile('third.jpg'), uploadFile('fourth.jpg')]);
    workflow.setItemTags(2, ['shared', 'local']);

    expect(workflow.applyStagedTags('add', ['bulk', 'shared'])).toBe(true);
    expect(workflow.items.map((item) => item.tags)).toEqual([
      ['shared', 'initial'],
      ['shared', 'initial'],
      ['shared', 'local', 'bulk'],
      ['shared', 'initial', 'bulk']
    ]);

    expect(workflow.applyStagedTags('remove', ['shared'])).toBe(true);
    expect(workflow.items.slice(2).map((item) => item.tags)).toEqual([
      ['local', 'bulk'],
      ['initial', 'bulk']
    ]);
    expect(workflow.stagedTagCandidates).toEqual([
      { name: 'local', count: 1 },
      { name: 'bulk', count: 2 },
      { name: 'initial', count: 1 }
    ]);

    expect(workflow.applyStagedTags('set', [])).toBe(true);
    expect(workflow.items.slice(2).map((item) => item.tags)).toEqual([[], []]);
    expect(workflow.items.slice(0, 2).map((item) => item.tags)).toEqual([
      ['shared', 'initial'],
      ['shared', 'initial']
    ]);
    expect(workflow.stagedTagCandidates).toEqual([]);
  });

  it('maintains staged completion counts incrementally through edits, removal, and admission', async () => {
    const workflow = createUploadWorkflow();
    workflow.tags = 'shared';
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    expect(workflow.stagedTagCandidates).toEqual([{ name: 'shared', count: 2 }]);

    workflow.setItemTags(0, ['shared', 'local:first']);
    expect(workflow.stagedTagCandidates).toEqual([
      { name: 'shared', count: 2 },
      { name: 'local:first', count: 1 }
    ]);

    workflow.removeAt(1);
    expect(workflow.stagedTagCandidates).toEqual([
      { name: 'shared', count: 1 },
      { name: 'local:first', count: 1 }
    ]);

    await workflow.submit(async () => pendingJob('job-staged-counts', 0, 1));
    expect(workflow.stagedTagCandidates).toEqual([]);
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
        ['uploading', 74],
        ['uploading', 0]
      ]);
      return pendingJob('job-batch');
    });

    expect(calls).toBe(1);
    expect(workflow.files).toEqual([]);
    expect(workflow.activeJobIDs).toEqual(['job-batch']);
    expect(workflow.items.map((item) => [item.status, item.progress, item.batchID])).toEqual([
      ['queued', 100, 1],
      ['queued', 100, 1]
    ]);
    now.mockRestore();
  });

  it('keeps completed transport progress monotonic while durable import runs', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    await workflow.submit(async () => pendingJob('job-batch'));

    expect(workflow.applyJob(pendingJob('job-batch', 1, 2))).toEqual({ completed: false, changedFiles: false });
    expect(workflow.items.map((item) => [item.status, item.progress])).toEqual([
      ['importing', 100],
      ['importing', 100]
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
    expect(workflow.items.map((item) => [item.status, item.batchID])).toEqual([
      ['imported', 1],
      ['duplicate_existing', 1]
    ]);
  });

  it('marks the whole selection failed when the aggregate transport fails', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    await workflow.submit(async (variables) => {
      variables.onProgress?.(43);
      throw new Error('Network error while uploading files');
    });
    expect(workflow.items.map((item) => [item.status, item.progress])).toEqual([
      ['error', 86],
      ['error', 0]
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

  it('stages additional files while an older durable batch is active', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('queued.jpg')]);
    await workflow.submit(async () => pendingJob('job-first', 0, 1));

    workflow.select([uploadFile('later.jpg')]);

    expect(workflow.files.map((file) => file.name)).toEqual(['later.jpg']);
    expect(workflow.items.map((item) => [item.name, item.status, item.batchID])).toEqual([
      ['queued.jpg', 'queued', 1],
      ['later.jpg', 'staged', undefined]
    ]);
  });

  it('submits a second batch while the first browser request is still in flight', async () => {
    const workflow = createUploadWorkflow();
    let releaseFirst: ((job: Job & BackgroundOperation) => void) | undefined;
    const firstResponse = new Promise<Job & BackgroundOperation>((resolve) => { releaseFirst = resolve; });

    workflow.select([uploadFile('first.jpg')]);
    const firstSubmission = workflow.submit(async () => firstResponse);
    expect(workflow.busy).toBe(true);
    expect(workflow.files).toEqual([]);

    workflow.select([uploadFile('second.jpg')]);
    expect(workflow.files.map((file) => file.name)).toEqual(['second.jpg']);
    await workflow.submit(async () => pendingJob('job-second', 0, 1));

    expect(workflow.busy).toBe(true);
    expect(workflow.activeJobID).toBe('job-second');
    expect(workflow.items.map((item) => [item.name, item.batchID])).toEqual([
      ['first.jpg', 1],
      ['second.jpg', 2]
    ]);

    releaseFirst?.(pendingJob('job-first', 0, 1));
    await firstSubmission;

    expect(workflow.busy).toBe(false);
    expect(workflow.files).toEqual([]);
    expect(workflow.activeJobIDs).toEqual(['job-second', 'job-first']);
  });

  it('does not let an older job completion clear newer staged files', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg')]);
    await workflow.submit(async () => pendingJob('job-first', 0, 1));
    workflow.select([uploadFile('later.jpg')]);

    expect(workflow.applyJob(completedJob('job-first', 'first.jpg'))).toEqual({ completed: true, changedFiles: true });

    expect(workflow.files.map((file) => file.name)).toEqual(['later.jpg']);
    expect(workflow.items.map((item) => [item.name, item.status])).toEqual([
      ['first.jpg', 'imported'],
      ['later.jpg', 'staged']
    ]);
  });

  it('can clear newer staging while an older durable batch remains active', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg')]);
    await workflow.submit(async () => pendingJob('job-first', 0, 1));
    workflow.select([uploadFile('later.jpg')]);

    workflow.clear('staged');

    expect(workflow.files).toEqual([]);
    expect(workflow.items.map((item) => [item.name, item.status])).toEqual([
      ['first.jpg', 'queued']
    ]);
    expect(workflow.activeJobIDs).toEqual(['job-first']);
  });

  it('keeps completed rows visible while direct tag changes are still saving', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg')]);
    await workflow.submit(async () => ({
      files: [{ id: 'file-first', name: 'first.jpg', size: 10, target_id: 'default', status: 'uploaded' }]
    } as never));

    workflow.setItemTags(0, ['viewer:edited']);
    expect(workflow.items[0]?.tagSyncPending).toBe(true);

    workflow.clear('done');
    expect(workflow.items.map((item) => item.name)).toEqual(['first.jpg']);

    workflow.markItemTagSyncApplied(0, 'add', ['viewer:edited']);
    expect(workflow.items[0]?.tagSyncPending).toBe(false);
    workflow.clear('done');
    expect(workflow.items).toEqual([]);
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
