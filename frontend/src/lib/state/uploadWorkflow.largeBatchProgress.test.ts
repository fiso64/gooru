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

import { createUploadWorkflow, maxFilesPerMultipartUpload } from './uploadWorkflow.svelte';

function uploadFile(index: number): File {
  return {
    name: `file-${index}.jpg`,
    size: 10,
    type: 'image/jpeg',
    lastModified: 1_700_000_000_000 + index
  } as File;
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

describe('large upload transport progress', () => {
  beforeEach(() => untrackSpy.mockClear());

  it('mutates only rows whose displayed progress changes and ignores duplicate aggregate events per bounded segment', async () => {
    const workflow = createUploadWorkflow();
    workflow.select(Array.from({ length: 10_000 }, (_, index) => uploadFile(index)));
    let callIndex = 0;

    await workflow.submit(async (variables) => {
      callIndex += 1;
      expect(variables.files).toHaveLength(maxFilesPerMultipartUpload);
      expect(variables.segmentCount).toBe(10);
      expect(variables.segmentIndex).toBe(callIndex - 1);
      expect(variables.operationID).toBe(callIndex === 1 ? undefined : 'job-large');
      const assignSpy = vi.spyOn(Object, 'assign');
      try {
        assignSpy.mockClear();
        variables.onProgress?.(1);
        expect(assignSpy).toHaveBeenCalledTimes(10);

        variables.onProgress?.(1);
        expect(assignSpy).toHaveBeenCalledTimes(10);

        variables.onProgress?.(2);
        expect(assignSpy).toHaveBeenCalledTimes(20);
      } finally {
        assignSpy.mockRestore();
      }
      return pendingJob('job-large', 10);
    });

    expect(callIndex).toBe(10);
    expect(workflow.activeJobIDs).toEqual(['job-large']);
    expect(workflow.items).toHaveLength(10_000);
    expect(workflow.items.every((item) => item.status === 'queued')).toBe(true);
  });
});
