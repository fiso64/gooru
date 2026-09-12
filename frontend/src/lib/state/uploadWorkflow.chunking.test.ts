import { beforeEach, describe, expect, it, vi } from 'vitest';
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

import { createUploadWorkflow, maxFilesPerMultipartUpload } from './uploadWorkflow.svelte';

function uploadFile(index: number): File {
  return { name: `file-${index}.jpg`, size: 10, type: 'image/jpeg', lastModified: index } as File;
}

function pendingJob(id: string, total: number): Job & BackgroundOperation {
  return {
    id,
    type: 'upload_import',
    kind: 'upload_import',
    status: 'pending',
    progress: 0,
    progress_total: total,
    progress_completed: 0,
    progress_completed_prefix: 0,
    progress_failed: 0,
    submitted_at: '2026-09-12T00:00:00Z',
    created_at: '2026-09-12T00:00:00Z'
  } as Job & BackgroundOperation;
}

describe('createUploadWorkflow bounded multipart submissions', () => {
  beforeEach(() => untrackSpy.mockClear());

  it('splits a large selection into bounded requests while preserving global queue metadata', async () => {
    const workflow = createUploadWorkflow();
    const files = Array.from({ length: maxFilesPerMultipartUpload * 2 + 1 }, (_, index) => uploadFile(index));
    workflow.select(files);
    const calls: Array<{
      names: string[];
      queueIndex: number[];
      queueTotal: number[];
      queueTimeMs: number[];
      queueFirstTimeMs?: number;
      queueLastTimeMs?: number;
    }> = [];

    await workflow.submit(async (variables) => {
      calls.push({
        names: variables.files.map((file) => file.name),
        queueIndex: [...(variables.queueIndex ?? [])],
        queueTotal: [...(variables.queueTotal ?? [])],
        queueTimeMs: [...(variables.queueTimeMs ?? [])],
        queueFirstTimeMs: variables.queueFirstTimeMs,
        queueLastTimeMs: variables.queueLastTimeMs
      });
      return pendingJob(`job-${calls.length}`, variables.files.length);
    });

    expect(calls.map((call) => call.names.length)).toEqual([1000, 1000, 1]);
    expect(calls[0]?.queueIndex).toEqual(Array.from({ length: 1000 }, (_, index) => index));
    expect(calls[1]?.queueIndex).toEqual(Array.from({ length: 1000 }, (_, index) => index + 1000));
    expect(calls[2]?.queueIndex).toEqual([2000]);
    for (const call of calls) {
      expect(call.queueTotal).toEqual(Array.from({ length: call.names.length }, () => 2001));
      expect(call.queueTimeMs).toHaveLength(call.names.length);
      expect(call.queueFirstTimeMs).toBe(call.queueLastTimeMs);
    }
    expect(workflow.activeJobIDs).toEqual(['job-1', 'job-2', 'job-3']);
    expect(workflow.items.map((item) => item.status)).toEqual(Array.from({ length: 2001 }, () => 'queued'));
  });

  it('cancels admitted chunks and stops submitting later chunks', async () => {
    const workflow = createUploadWorkflow();
    workflow.select(Array.from({ length: maxFilesPerMultipartUpload * 2 + 1 }, (_, index) => uploadFile(index)));
    let callCount = 0;
    let resolveSecondStarted!: () => void;
    const secondStarted = new Promise<void>((resolve) => { resolveSecondStarted = resolve; });

    const submission = workflow.submit(async (variables) => {
      callCount += 1;
      if (callCount === 1) return pendingJob('job-first', variables.files.length);
      resolveSecondStarted();
      return new Promise<BackgroundOperation>((_resolve, reject) => {
        const rejectCanceled = () => reject(new ApiError(0, 'request_aborted', 'Upload was canceled'));
        if (variables.signal?.aborted) rejectCanceled();
        else variables.signal?.addEventListener('abort', rejectCanceled, { once: true });
      });
    });

    await secondStarted;
    const canceled: string[] = [];
    const cancellation = workflow.cancel(async (id) => {
      canceled.push(id);
      return { ...pendingJob(id, 1000), status: 'canceled' } as Job;
    });

    await Promise.all([submission, cancellation]);

    expect(callCount).toBe(2);
    expect(canceled).toEqual(['job-first']);
    expect(workflow.activeJobIDs).toEqual([]);
    expect(workflow.busy).toBe(false);
    expect(workflow.items.every((item) => item.status === 'canceled')).toBe(true);
  });
});
