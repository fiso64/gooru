import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Job } from '$lib/api/types';
import type { BackgroundOperation } from '$lib/api/operations';
import { ApiError } from '$lib/api/client';

const { untrackSpy } = vi.hoisted(() => ({
  untrackSpy: vi.fn((value: unknown) => typeof value === 'function' ? (value as () => unknown)() : value)
}));

vi.mock('svelte', async () => {
  const actual = await vi.importActual<typeof import('svelte')>('svelte');
  return { ...actual, untrack: untrackSpy };
});

import { createUploadWorkflow } from './uploadWorkflow.svelte';
import { uploadAdmissionFallbackMs } from '$lib/uploadBackpressure';

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

describe('server upload queue backpressure', () => {
  afterEach(() => vi.useRealTimers());

  it('does not busy-loop when the server queue is full outside the local window', async () => {
    vi.useFakeTimers();
    const workflow = createUploadWorkflow();
    workflow.select([{ name: 'one.jpg', size: 1, type: 'image/jpeg', lastModified: 0 } as File]);
    let calls = 0;

    const submission = workflow.submit(async () => {
      calls += 1;
      if (calls === 1) throw new ApiError(503, 'job_queue_full', 'job queue is full');
      return pendingJob('job-one');
    });

    // Flush the initial async submission without advancing fake wall-clock time.
    // vi.waitFor advances fake timers while polling, which made this exact
    // fallback-boundary assertion consume part of the 250 ms timeout itself.
    await vi.advanceTimersByTimeAsync(0);
    expect(calls).toBe(1);
    await vi.advanceTimersByTimeAsync(uploadAdmissionFallbackMs - 1);
    expect(calls).toBe(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(calls).toBe(2);
    await submission;
  });
});
