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

  it('preserves distinct prior state for duplicate filenames', () => {
    const firstDuplicate = {
      ...queuedItem('same.jpg'),
      tags: ['row:first'],
      batchID: 11,
      queueTimeMs: 101
    };
    const other = {
      ...queuedItem('other.jpg'),
      tags: ['row:other'],
      batchID: 12,
      queueTimeMs: 202
    };
    const secondDuplicate = {
      ...queuedItem('same.jpg'),
      tags: ['row:second'],
      batchID: 13,
      queueTimeMs: 303
    };
    const response = {
      affected_count: 3,
      files: [
        { id: 'file-other', name: 'other.jpg', size: 10, target_id: 'primary', status: 'imported' },
        { id: 'file-same-first', name: 'same.jpg', size: 10, target_id: 'primary', status: 'imported' },
        { id: 'file-same-second', name: 'same.jpg', size: 10, target_id: 'primary', status: 'imported' }
      ]
    } as UploadImportResponse;

    const items = itemsFromResult(response, [firstDuplicate, other, secondDuplicate]);

    expect(items.map((item) => ({
      name: item.name,
      tags: item.tags,
      batchID: item.batchID,
      queueTimeMs: item.queueTimeMs,
      remoteFileID: item.remoteFileID
    }))).toEqual([
      { name: 'other.jpg', tags: ['row:other'], batchID: 12, queueTimeMs: 202, remoteFileID: 'file-other' },
      { name: 'same.jpg', tags: ['row:first'], batchID: 11, queueTimeMs: 101, remoteFileID: 'file-same-first' },
      { name: 'same.jpg', tags: ['row:second'], batchID: 13, queueTimeMs: 303, remoteFileID: 'file-same-second' }
    ]);
  });

  it('does not reuse fallback state after a server-side rename', () => {
    const first = {
      ...queuedItem('same.jpg'),
      tags: ['row:first'],
      batchID: 21
    };
    const second = {
      ...queuedItem('same.jpg'),
      tags: ['row:second'],
      batchID: 22
    };
    const response = {
      affected_count: 2,
      files: [
        { id: 'file-renamed', name: 'same-1.jpg', size: 10, target_id: 'primary', status: 'imported' },
        { id: 'file-original', name: 'same.jpg', size: 10, target_id: 'primary', status: 'imported' }
      ]
    } as UploadImportResponse;

    const items = itemsFromResult(response, [first, second]);

    expect(items.map((item) => ({ name: item.name, tags: item.tags, batchID: item.batchID }))).toEqual([
      { name: 'same-1.jpg', tags: ['row:first'], batchID: 21 },
      { name: 'same.jpg', tags: ['row:second'], batchID: 22 }
    ]);
  });
});
