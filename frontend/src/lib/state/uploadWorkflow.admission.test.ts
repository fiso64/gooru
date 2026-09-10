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

import { browserUploadConcurrency, createUploadWorkflow, uploadJobStatusBatchSize } from './uploadWorkflow.svelte';

function uploadFile(index: number): File {
  return { name: `file-${index}.jpg`, size: 10, type: 'image/jpeg', lastModified: 0 } as File;
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
    submitted_at: '2026-09-07T00:00:00Z',
    created_at: '2026-09-07T00:00:00Z'
  } as Job & BackgroundOperation;
}

function completedJob(id: string): Job {
  return {
    ...pendingJob(id),
    status: 'completed',
    progress: 1,
    result: { files: [{ name: id.replace(/^job-/, ''), size: 10, target_id: 'default', status: 'imported' }] }
  } as Job;
}

describe('upload admission backpressure', () => {
  beforeEach(() => untrackSpy.mockClear());

  it('wakes blocked browser workers as soon as tracked jobs complete', async () => {
    const workflow = createUploadWorkflow();
    const total = uploadJobStatusBatchSize + browserUploadConcurrency + 2;
    workflow.select(Array.from({ length: total }, (_, index) => uploadFile(index + 1)));
    let calls = 0;

    const submission = workflow.submit(async (variables) => {
      calls += 1;
      return pendingJob(`job-${variables.files[0].name}`);
    });

    await vi.waitFor(() => {
      expect(workflow.activeJobIDs.length).toBeGreaterThanOrEqual(uploadJobStatusBatchSize);
      expect(calls).toBeLessThan(total);
    });
    // Let already-admitted browser transfers settle before measuring the block.
    await new Promise((resolve) => setTimeout(resolve, 0));
    const blockedCalls = calls;
    const active = workflow.activeJobIDs;
    expect(blockedCalls).toBeLessThan(total);

    // Up to four transfers can already be in flight when the accepted-job
    // window reaches 64, so release exactly enough tracked jobs to reopen it.
    const releases = Math.max(1, active.length - uploadJobStatusBatchSize + 1);
    for (const id of active.slice(0, releases)) workflow.applyJob(completedJob(id));

    // The previous fixed 250 ms admission sleep cannot satisfy this bound.
    await vi.waitFor(() => expect(calls).toBeGreaterThan(blockedCalls), { timeout: 150 });

    for (const id of [...workflow.activeJobIDs]) workflow.applyJob(completedJob(id));
    await submission;
  });
});
