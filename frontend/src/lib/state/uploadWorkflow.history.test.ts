import { describe, expect, it } from 'vitest';
import type { UploadImportResponse } from '$lib/api/types';
import { createUploadWorkflow } from './uploadWorkflow.svelte';

function uploadFile(name: string): File {
  return { name, size: 10, type: 'image/jpeg', lastModified: 0 } as File;
}

function uploaded(name: string): UploadImportResponse {
  return {
    files: [{ name, size: 10, target_id: '', status: 'uploaded' }]
  } as UploadImportResponse;
}

describe('upload workflow history', () => {
  it('keeps completed rows terminal when a later batch is uploaded', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg')]);
    await workflow.submit(async () => uploaded('first.jpg'));

    expect(workflow.files).toEqual([]);
    expect(workflow.items.map((item) => [item.name, item.status])).toEqual([
      ['first.jpg', 'uploaded']
    ]);

    workflow.select([uploadFile('second.jpg')]);
    expect(workflow.items.map((item) => [item.name, item.status])).toEqual([
      ['first.jpg', 'uploaded'],
      ['second.jpg', 'staged']
    ]);

    const submitted: string[] = [];
    await workflow.submit(async (variables) => {
      const name = variables.files[0].name;
      submitted.push(name);
      return uploaded(name);
    });

    expect(submitted).toEqual(['second.jpg']);
    expect(workflow.items.map((item) => [item.name, item.status])).toEqual([
      ['first.jpg', 'uploaded'],
      ['second.jpg', 'uploaded']
    ]);
  });

  it('removes the matching staged file when completed history precedes it', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg')]);
    await workflow.submit(async () => uploaded('first.jpg'));
    workflow.select([uploadFile('second.jpg'), uploadFile('third.jpg')]);

    workflow.removeAt(1);

    expect(workflow.files.map((file) => file.name)).toEqual(['third.jpg']);
    expect(workflow.items.map((item) => [item.name, item.status])).toEqual([
      ['first.jpg', 'uploaded'],
      ['third.jpg', 'staged']
    ]);
  });
});
