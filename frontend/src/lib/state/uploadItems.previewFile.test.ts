import { describe, expect, it } from 'vitest';
import type { UploadImportResponse } from '$lib/api/types';
import { itemsFromResult, stagedUploadItems } from './uploadItems';

function resultFor(file: File, id?: string): UploadImportResponse {
  return {
    files: [{
      id,
      name: file.name,
      size: file.size,
      target_id: 'target',
      status: 'imported'
    }]
  } as UploadImportResponse;
}

describe('completed upload local previews', () => {
  it('preserves the local File when the completed result has remote identity', () => {
    const file = new File(['payload'], 'photo.jpg', { type: 'image/jpeg' });
    const [item] = itemsFromResult(resultFor(file, 'file-1'), stagedUploadItems([file]));

    expect(item?.remoteFileID).toBe('file-1');
    expect(item?.previewFile).toBe(file);
  });

  it('preserves the local File when the completed result has no remote identity', () => {
    const file = new File(['payload'], 'photo.jpg', { type: 'image/jpeg' });
    const [item] = itemsFromResult(resultFor(file), stagedUploadItems([file]));

    expect(item?.remoteFileID).toBeUndefined();
    expect(item?.previewFile).toBe(file);
  });
});
