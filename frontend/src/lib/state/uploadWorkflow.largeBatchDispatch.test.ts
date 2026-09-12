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

describe('large upload dispatch', () => {
  beforeEach(() => untrackSpy.mockClear());

  it('reaches the transport without 10k per-row reactive mutations', async () => {
    const workflow = createUploadWorkflow();
    workflow.select(Array.from({ length: 10_000 }, (_, index) => uploadFile(index)));

    const assignSpy = vi.spyOn(Object, 'assign');
    try {
      await workflow.submit(async () => {
        expect(assignSpy).toHaveBeenCalledTimes(0);
        expect(workflow.items).toHaveLength(10_000);
        expect(workflow.items.every((item) => item.status === 'uploading' && item.batchID === 1)).toBe(true);
        return pendingJob('job-large');
      });
    } finally {
      assignSpy.mockRestore();
    }

    expect(workflow.activeJobIDs).toEqual(['job-large']);
  });
});
