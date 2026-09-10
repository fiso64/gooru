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

import { browserUploadConcurrency, createUploadWorkflow } from './uploadWorkflow.svelte';

function uploadFile(name: string): File {
  return { name, size: 10, type: 'image/jpeg', lastModified: 0 } as File;
}

function pendingJob(id: string): Job & BackgroundOperation {
  return {
    id,
    type: 'upload_import',
    kind: 'upload_import',
    status: 'pending',
    progress: 0,
    progress_total: 1,
    progress_completed: 0,
    progress_failed: 0,
    submitted_at: '2026-09-01T00:00:00Z',
    created_at: '2026-09-01T00:00:00Z'
  } as Job & BackgroundOperation;
}

describe('createUploadWorkflow', () => {
  beforeEach(() => untrackSpy.mockClear());

  it('defaults browser uploads to rename conflicts', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg')]);
    let policy = '';

    await workflow.submit(async (variables) => {
      policy = variables.conflictPolicy;
      return pendingJob('job-first');
    });

    expect(policy).toBe('rename');
  });

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

  it('captures queue metadata before parallel workers and preserves it across target changes', async () => {
    const now = vi.spyOn(Date, 'now');
    now.mockReturnValueOnce(1_700_000_000_000).mockReturnValueOnce(1_700_000_010_000).mockReturnValue(1_800_000_000_000);
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg')]);
    workflow.select([uploadFile('second.jpg')]);
    workflow.setTarget('archive', 'reverse_queue');

    const calls: Array<{ name: string; queueTimeMs: number; queueFirstTimeMs: number; queueLastTimeMs: number; queueIndex: number; queueTotal: number; strategy: string }> = [];
    await workflow.submit(async (variables) => {
      calls.push({
        name: variables.files[0].name,
        queueTimeMs: variables.queueTimeMs,
        queueFirstTimeMs: variables.queueFirstTimeMs,
        queueLastTimeMs: variables.queueLastTimeMs,
        queueIndex: variables.queueIndex,
        queueTotal: variables.queueTotal,
        strategy: variables.addedAtStrategy
      });
      return pendingJob(`job-${variables.files[0].name}`);
    });

    expect(calls).toEqual([
      { name: 'first.jpg', queueTimeMs: 1_700_000_000_000, queueFirstTimeMs: 1_700_000_000_000, queueLastTimeMs: 1_700_000_010_000, queueIndex: 0, queueTotal: 2, strategy: 'reverse_queue' },
      { name: 'second.jpg', queueTimeMs: 1_700_000_010_000, queueFirstTimeMs: 1_700_000_000_000, queueLastTimeMs: 1_700_000_010_000, queueIndex: 1, queueTotal: 2, strategy: 'reverse_queue' }
    ]);
    expect(workflow.items.map((item) => item.queueTimeMs)).toEqual([1_700_000_000_000, 1_700_000_010_000]);
    now.mockRestore();
  });

  it('uploads each file independently with real per-file progress', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    const calls: string[] = [];

    await workflow.submit(async (variables) => {
      const name = variables.files[0].name;
      calls.push(name);
      expect(variables.files).toHaveLength(1);
      variables.onProgress?.(name === 'first.jpg' ? 37 : 64);
      return pendingJob(`job-${name}`);
    });

    expect(calls).toEqual(['first.jpg', 'second.jpg']);
    expect(workflow.items.map((item) => [item.status, item.progress])).toEqual([
      ['queued', 100],
      ['queued', 100]
    ]);
    expect(workflow.activeJobIDs).toEqual(['job-first.jpg', 'job-second.jpg']);
  });

  it('runs at most four browser transfers and starts queued siblings as slots free', async () => {
    const workflow = createUploadWorkflow();
    const names = Array.from({ length: 7 }, (_, index) => `file-${index + 1}.jpg`);
    workflow.select(names.map(uploadFile));

    let active = 0;
    let maxActive = 0;
    const started: string[] = [];
    const release = new Map<string, () => void>();

    const submission = workflow.submit(async (variables) => {
      const name = variables.files[0].name;
      active += 1;
      maxActive = Math.max(maxActive, active);
      started.push(name);
      await new Promise<void>((resolve) => release.set(name, resolve));
      active -= 1;
      return pendingJob(`job-${name}`);
    });

    await vi.waitFor(() => expect(started).toHaveLength(browserUploadConcurrency));
    expect(started).toEqual(names.slice(0, browserUploadConcurrency));
    expect(active).toBe(browserUploadConcurrency);
    expect(maxActive).toBe(browserUploadConcurrency);

    release.get('file-2.jpg')?.();
    await vi.waitFor(() => expect(started).toHaveLength(5));
    expect(started[4]).toBe('file-5.jpg');
    expect(active).toBe(browserUploadConcurrency);

    release.get('file-1.jpg')?.();
    await vi.waitFor(() => expect(started).toHaveLength(6));
    expect(started[5]).toBe('file-6.jpg');
    expect(active).toBe(browserUploadConcurrency);

    release.get('file-4.jpg')?.();
    await vi.waitFor(() => expect(started).toHaveLength(7));
    expect(started[6]).toBe('file-7.jpg');
    expect(active).toBe(browserUploadConcurrency);

    for (const name of names.slice(2)) release.get(name)?.();
    await submission;

    expect(maxActive).toBe(browserUploadConcurrency);
    expect(workflow.items.every((item) => item.status === 'queued')).toBe(true);
    expect(workflow.activeJobIDs).toHaveLength(names.length);
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

  it('applies each tracked import job independently while polling the batch together', async () => {
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
    expect(workflow.activeJobID).toBe('job-first.jpg,job-second.jpg');
  });

  it('applies multiple completed jobs from one status response', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    await workflow.submit(async (variables) => pendingJob(`job-${variables.files[0].name}`));

    const completed = (name: string) => ({
      ...pendingJob(`job-${name}`),
      status: 'completed',
      result: { files: [{ name, size: 10, target_id: '', status: 'imported' }] }
    }) as Job;

    expect(workflow.applyJob({ items: [completed('first.jpg'), completed('second.jpg')] })).toEqual({ completed: true, changedFiles: true });
    expect(workflow.activeJobIDs).toEqual([]);
    expect(workflow.items.map((item) => item.status)).toEqual(['imported', 'imported']);
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

  it('preserves initial tags after a completed upload batch', async () => {
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
