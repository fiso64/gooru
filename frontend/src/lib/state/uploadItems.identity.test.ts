import { describe, expect, it } from 'vitest';
import { itemsFromJob, itemsFromResult, type UploadItem } from './uploadItems';
import type { Job, UploadImportResponse } from '$lib/api/types';

function queuedItem(name: string): UploadItem {
  return {
    name,
    size: 10,
    type: 'image/jpeg',
    tags: ['person:alice'],
    status: 'queued',
    progress: 100
  };
}

function result(status: 'imported' | 'duplicate_existing', id: string): UploadImportResponse {
  return {
    affected_count: status === 'imported' ? 1 : 0,
    files: [{
      id,
      name: 'first.jpg',
      size: 10,
      target_id: 'primary',
      status
    }]
  } as UploadImportResponse;
}

describe('upload result remote identities', () => {
  it('retains the imported file id while preserving desired row tags', () => {
    const [item] = itemsFromResult(result('imported', 'file-imported'), [queuedItem('first.jpg')]);

    expect(item.remoteFileID).toBe('file-imported');
    expect(item.tags).toEqual(['person:alice']);
    expect(item.status).toBe('imported');
  });

  it('retains the existing library file id for duplicate content', () => {
    const [item] = itemsFromResult(result('duplicate_existing', 'file-existing'), [queuedItem('first.jpg')]);

    expect(item.remoteFileID).toBe('file-existing');
    expect(item.status).toBe('duplicate_existing');
  });

  it('propagates ids from a completed durable job result', () => {
    const job = {
      id: 'upload-one',
      type: 'upload_import',
      status: 'completed',
      progress_total: 1,
      progress_completed: 1,
      progress_failed: 0,
      result: result('imported', 'file-job-result'),
      submitted_at: '2026-09-14T00:00:00Z'
    } as Job;

    const [item] = itemsFromJob([queuedItem('first.jpg')], job);
    expect(item.remoteFileID).toBe('file-job-result');
    expect(item.tags).toEqual(['person:alice']);
  });
});
