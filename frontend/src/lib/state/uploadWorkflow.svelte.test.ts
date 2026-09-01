import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Job } from '$lib/api/types';

const { untrackSpy } = vi.hoisted(() => ({
  untrackSpy: vi.fn((value: unknown) => typeof value === 'function' ? (value as () => unknown)() : value)
}));

vi.mock('svelte', async () => {
  const actual = await vi.importActual<typeof import('svelte')>('svelte');
  return { ...actual, untrack: untrackSpy };
});

import { createUploadWorkflow } from './uploadWorkflow.svelte';

describe('createUploadWorkflow', () => {
  beforeEach(() => untrackSpy.mockClear());

  it('applies polled jobs outside the caller reactive dependency graph', () => {
    const workflow = createUploadWorkflow();
    const job = {
      id: 'job-1',
      type: 'upload_import',
      status: 'running',
      submitted_at: '2026-09-01T00:00:00Z',
      progress: 0.5
    } as Job;

    expect(workflow.applyJob(job)).toEqual({ completed: false, changedFiles: false });
    expect(untrackSpy).toHaveBeenCalledWith(expect.any(Function));
    expect(workflow.status).toBe('Importing');
  });
});
