import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Job } from '$lib/api/types';
import type { BackgroundOperation } from '$lib/api/operations';
import { ApiError } from '$lib/api/client';
import { uploadAdmissionFallbackMs } from '$lib/uploadBackpressure';

const { untrackSpy } = vi.hoisted(() => ({
  untrackSpy: vi.fn((value: unknown) => typeof value === 'function' ? (value as () => unknown)() : value)
}));

vi.mock('svelte', async () => {
  const actual = await vi.importActual<typeof import('svelte')>('svelte');
  return { ...actual, untrack: untrackSpy };
});

import { createUploadWorkflow } from './uploadWorkflow.svelte';

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
    progress_total: 3,
    progress_completed: 0,
    progress_failed: 0,
    submitted_at: '2026-09-11T00:00:00Z',
    created_at: '2026-09-11T00:00:00Z'
  } as Job & BackgroundOperation;
}

describe('aggregate upload admission backpressure', () => {
  beforeEach(() => {
    untrackSpy.mockClear();
    vi.useFakeTimers();
  });

  it('retries the same aggregate batch after durable admission is full', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile(1), uploadFile(2), uploadFile(3)]);
    let calls = 0;
    const submission = workflow.submit(async (variables) => {
      calls += 1;
      expect(variables.files).toHaveLength(3);
      if (calls === 1) throw new ApiError(503, 'job_queue_full', 'job queue is full');
      return pendingJob('job-batch');
    });

    await vi.advanceTimersByTimeAsync(uploadAdmissionFallbackMs);
    await submission;
    expect(calls).toBe(2);
    expect(workflow.activeJobIDs).toEqual(['job-batch']);
    vi.useRealTimers();
  });
});
