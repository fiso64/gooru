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

function pendingJob(id: string): Job & BackgroundOperation {
  return {
    id,
    type: 'upload_import',
    kind: 'upload_import',
    status: 'pending',
    progress: 0,
    progress_total: 1,
    progress_completed: 0,
    progress_completed_prefix: 0,
    progress_failed: 0,
    submitted_at: '2026-09-12T00:00:00Z',
    created_at: '2026-09-12T00:00:00Z'
  } as Job & BackgroundOperation;
}

function canceledJob(id: string): Job {
  return { ...pendingJob(id), status: 'canceled' } as Job;
}

describe('createUploadWorkflow cancellation across concurrent admissions', () => {
  beforeEach(() => untrackSpy.mockClear());

  it('cancels an operation that is admitted after queue cancellation starts', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg')]);
    await workflow.submit(async () => pendingJob('job-first'));

    let resolveSecond!: (job: Job & BackgroundOperation) => void;
    let markSecondStarted!: () => void;
    const secondStarted = new Promise<void>((resolve) => { markSecondStarted = resolve; });
    const secondResponse = new Promise<Job & BackgroundOperation>((resolve) => { resolveSecond = resolve; });

    workflow.select([uploadFile('second.jpg')]);
    const secondSubmission = workflow.submit(async () => {
      markSecondStarted();
      return secondResponse;
    });
    await secondStarted;

    const canceled: string[] = [];
    const cancelResult = await workflow.cancel(async (id) => {
      canceled.push(id);
      return canceledJob(id);
    });

    expect(cancelResult).toEqual({ changed: true });
    expect(canceled).toEqual(['job-first']);
    expect(workflow.cancelBusy).toBe(true);

    resolveSecond(pendingJob('job-second'));
    const submitResult = await secondSubmission;

    expect(submitResult).toEqual({ queued: false, changedFiles: false });
    expect(canceled).toEqual(['job-first', 'job-second']);
    expect(workflow.activeJobIDs).toEqual([]);
    expect(workflow.items.map((item) => item.status)).toEqual(['canceled', 'canceled']);
    expect(workflow.busy).toBe(false);
    expect(workflow.cancelBusy).toBe(false);
  });

  it('keeps a late-admitted operation tracked when its cancellation request fails', async () => {
    const workflow = createUploadWorkflow();
    let resolveUpload!: (job: Job & BackgroundOperation) => void;
    let markStarted!: () => void;
    const started = new Promise<void>((resolve) => { markStarted = resolve; });
    const response = new Promise<Job & BackgroundOperation>((resolve) => { resolveUpload = resolve; });

    workflow.select([uploadFile('late.jpg')]);
    const submission = workflow.submit(async () => {
      markStarted();
      return response;
    });
    await started;

    expect(await workflow.cancel(async () => { throw new Error('cancel failed'); })).toEqual({ changed: true });
    resolveUpload(pendingJob('job-late'));
    const submitResult = await submission;

    expect(submitResult).toEqual({ queued: true, changedFiles: false });
    expect(workflow.activeJobIDs).toEqual(['job-late']);
    expect(workflow.items[0]?.status).toBe('queued');
    expect(workflow.items[0]?.error).toContain('cancel failed');
    expect(workflow.cancelBusy).toBe(false);
  });
});
