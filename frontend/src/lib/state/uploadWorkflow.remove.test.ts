import { describe, expect, it, vi } from 'vitest';

vi.mock('svelte', async () => {
  const actual = await vi.importActual<typeof import('svelte')>('svelte');
  return { ...actual, untrack: (value: unknown) => typeof value === 'function' ? (value as () => unknown)() : value };
});

import { createUploadWorkflow } from './uploadWorkflow.svelte';

function uploadFile(name: string): File {
  return { name, size: 10, type: 'image/jpeg', lastModified: 0 } as File;
}

describe('upload workflow row removal', () => {
  it('recomputes the queue summary as completed result rows are removed', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);

    await workflow.submit(async () => ({
      affected_count: 2,
      files: [
        { id: 'file-first', name: 'first.jpg', size: 10, target_id: 'default', status: 'imported' },
        { id: 'file-second', name: 'second.jpg', size: 10, target_id: 'default', status: 'imported' }
      ]
    } as never));

    expect(workflow.status).toBe('2 imported');
    workflow.removeAt(0);
    expect(workflow.items.map((item) => item.name)).toEqual(['second.jpg']);
    expect(workflow.status).toBe('1 imported');

    workflow.removeAt(0);
    expect(workflow.items).toEqual([]);
    expect(workflow.status).toBe('');
  });
});
