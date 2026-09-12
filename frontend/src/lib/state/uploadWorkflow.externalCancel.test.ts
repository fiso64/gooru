import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '$lib/api/client';

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

describe('createUploadWorkflow external cancellation', () => {
  beforeEach(() => untrackSpy.mockClear());

  it('shows a durable cancellation as canceled even when the local controller did not initiate it', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);

    const result = await workflow.submit(async () => {
      throw new ApiError(0, 'request_aborted', 'Upload was canceled');
    });

    expect(result).toEqual({ queued: false, changedFiles: false });
    expect(workflow.items.map((item) => item.status)).toEqual(['canceled', 'canceled']);
    expect(workflow.items.map((item) => item.error)).toEqual(['', '']);
    expect(workflow.status.toLowerCase()).toContain('canceled');
    expect(workflow.busy).toBe(false);
  });
});
