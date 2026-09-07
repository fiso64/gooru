import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Job } from '$lib/api/types';

const { untrackSpy } = vi.hoisted(() => ({
  untrackSpy: vi.fn((value: unknown) => typeof value === 'function' ? (value as () => unknown)() : value)
}));

vi.mock('svelte', async () => {
  const actual = await vi.importActual<typeof import('svelte')>('svelte');
  return { ...actual, untrack: untrackSpy };
});

import { createUploadWorkflow, uploadJobStatusBatchSize } from './uploadWorkflow.svelte';

function uploadFile(index: number): File {
  return { name: `file-${index}.jpg`, size: 10, type: 'image/jpeg', lastModified: 0 } as File;
}

function pendingJob(id: string): Job {
  return {
    id,
    type: 'upload_import',
    status: 'pending',
    submitted_at: '2026-09-07T00:00:00Z'
  } as Job;
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

  it('wakes a blocked browser worker as soon as a tracked job completes', async () => {
    const workflow = createUploadWorkflow();
    const total = uploadJobStatusBatchSize + 1;
    workflow.select(Array.from({ length: total }, (_, index) => uploadFile(index + 1)));
    let calls = 0;

    const submission = workflow.submit(async (variables) => {
      calls += 1;
      return pendingJob(`job-${variables.files[0].name}`);
    });

    await vi.waitFor(() => expect(workflow.activeJobIDs).toHaveLength(uploadJobStatusBatchSize));
    expect(calls).toBe(uploadJobStatusBatchSize);

    workflow.applyJob(completedJob(workflow.activeJobIDs[0]));

    // The previous fixed 250 ms admission sleep cannot satisfy this bound.
    await vi.waitFor(() => expect(calls).toBe(total), { timeout: 150 });
    await submission;
  });
});
