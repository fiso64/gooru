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

function uploadFile(index: number): File {
  return {
    name: `file-${index}.jpg`,
    size: 10,
    type: 'image/jpeg',
    lastModified: 1_700_000_000_000 + index
  } as File;
}

function pendingJob(id: string): Job & BackgroundOperation {
  return {
    id,
    type: 'upload_import',
    kind: 'upload_import',
    status: 'pending',
    progress: 0,
    progress_total: 10_000,
    progress_completed: 0,
    progress_completed_prefix: 0,
    progress_failed: 0,
    submitted_at: '2026-09-12T00:00:00Z',
    created_at: '2026-09-12T00:00:00Z'
  } as Job & BackgroundOperation;
}

describe('large upload transport progress', () => {
  beforeEach(() => untrackSpy.mockClear());

  it('mutates only rows whose displayed progress changes and ignores duplicate aggregate events', async () => {
    const workflow = createUploadWorkflow();
    workflow.select(Array.from({ length: 10_000 }, (_, index) => uploadFile(index)));

    await workflow.submit(async (variables) => {
      const assignSpy = vi.spyOn(Object, 'assign');
      try {
        assignSpy.mockClear();
        variables.onProgress?.(1);
        expect(assignSpy).toHaveBeenCalledTimes(100);
        expect(workflow.items.filter((item) => item.progress === 100)).toHaveLength(100);

        variables.onProgress?.(1);
        expect(assignSpy).toHaveBeenCalledTimes(100);

        variables.onProgress?.(2);
        expect(assignSpy).toHaveBeenCalledTimes(200);
        expect(workflow.items.filter((item) => item.progress === 100)).toHaveLength(200);
      } finally {
        assignSpy.mockRestore();
      }
      return pendingJob('job-large');
    });

    expect(workflow.activeJobIDs).toEqual(['job-large']);
    expect(workflow.items).toHaveLength(10_000);
  });
});
