import { describe, expect, it } from 'vitest';
import {
  itemsFromResult,
  markUploadItemTagSyncAppliedInPlace,
  markUploadItemTagSyncErrorInPlace,
  setUploadItemTagsInPlace,
  type UploadItem
} from './uploadItems';
import type { UploadImportResponse } from '$lib/api/types';

function item(status: UploadItem['status'], remoteFileID?: string): UploadItem {
  return {
    name: 'first.jpg',
    size: 10,
    type: 'image/jpeg',
    tags: ['person:alice'],
    remoteFileID,
    status,
    progress: status === 'staged' ? 0 : 100
  };
}

function importedResult(id: string): UploadImportResponse {
  return {
    affected_count: 1,
    files: [{ id, name: 'first.jpg', size: 10, target_id: 'primary', status: 'imported' }]
  } as UploadImportResponse;
}

describe('upload item tag reconciliation state', () => {
  it('does not schedule server reconciliation for staged edits', () => {
    const items = [item('staged')];
    setUploadItemTagsInPlace(items, 0, ['person:bob']);
    expect(items[0].tags).toEqual(['person:bob']);
    expect(items[0].tagSyncPending).toBe(false);
  });

  it('marks explicit post-admission edits pending before a remote id exists', () => {
    const items = [item('queued')];
    setUploadItemTagsInPlace(items, 0, ['person:bob']);
    expect(items[0].tagSyncPending).toBe(true);

    const [completed] = itemsFromResult(importedResult('file-one'), items);
    expect(completed.remoteFileID).toBe('file-one');
    expect(completed.tags).toEqual(['person:bob']);
    expect(completed.tagSyncPending).toBe(true);
  });

  it('marks terminal edits pending immediately', () => {
    const items = [item('duplicate_existing', 'file-existing')];
    setUploadItemTagsInPlace(items, 0, ['rating:safe']);
    expect(items[0].tagSyncPending).toBe(true);
  });

  it('keeps a newer edit pending when an older add completes', () => {
    const items = [item('imported', 'file-one')];
    setUploadItemTagsInPlace(items, 0, ['person:bob']);
    setUploadItemTagsInPlace(items, 0, ['person:carol']);

    markUploadItemTagSyncAppliedInPlace(items, 0, 'add', ['person:bob']);
    expect(items[0].tagSyncPending).toBe(true);
    expect(items[0].tags).toEqual(['person:carol']);
    expect(items[0].tagSyncBaseTags).toEqual(['person:alice', 'person:bob']);
  });

  it('records a failed direct tag update without retry-looping automatically', () => {
    const items = [item('imported', 'file-one')];
    setUploadItemTagsInPlace(items, 0, ['person:bob']);
    markUploadItemTagSyncErrorInPlace(items, 0, 'Tag update failed');

    expect(items[0].tagSyncPending).toBe(false);
    expect(items[0].tagSyncError).toBe('Tag update failed');

    setUploadItemTagsInPlace(items, 0, ['person:carol']);
    expect(items[0].tagSyncPending).toBe(true);
    expect(items[0].tagSyncError).toBe('');
  });
});
