import { describe, expect, it } from 'vitest';
import type { Job } from '$lib/api/types';
import {
  countUploadStatuses,
  effectiveUploadTargetID,
  itemsFromJob,
  itemsFromResult,
  markUploadItemTagSyncAppliedInPlace,
  markUploadItemTagSyncErrorInPlace,
  rebaseUploadItemTagsFromRemoteInPlace,
  replaceUploadItemInPlace,
  retargetStagedUploadItems,
  setUploadItemTagsInPlace,
  stagedUploadItems,
  uploadItemTagSyncDelta,
  uploadProgressItem,
  uploadSummaryFromCounts,
  type UploadItem
} from './uploadItems';

const targets = [
  { id: 'primary', name: 'Primary' },
  { id: 'archive', name: 'Archive' }
];

function queueItem(index: number): UploadItem {
  return {
    name: `file-${index}.jpg`,
    size: 1024,
    type: 'image/jpeg',
    status: 'waiting',
    progress: 0
  };
}

function uploadFile(name: string): File {
  return { name, size: 10, type: 'image/jpeg', lastModified: 0 } as File;
}

describe('effectiveUploadTargetID', () => {
  it('uses the first configured target when no explicit target is selected', () => {
    expect(effectiveUploadTargetID('', targets)).toBe('primary');
  });

  it('preserves an explicit configured target', () => {
    expect(effectiveUploadTargetID('archive', targets)).toBe('archive');
  });

  it('falls back when a previously selected target disappears', () => {
    expect(effectiveUploadTargetID('removed', targets)).toBe('primary');
  });

  it('returns empty when uploads have no configured targets', () => {
    expect(effectiveUploadTargetID('', [])).toBe('');
  });
});

describe('per-item upload tags', () => {
  it('snapshots and normalizes tags when a row is staged', () => {
    const items = stagedUploadItems(
      [uploadFile('first.jpg')],
      'primary',
      123,
      ['source:upload', ' ', 'source:upload', ' rating:safe ']
    );

    expect(items[0].tags).toEqual(['source:upload', 'rating:safe']);
  });

  it('retargets staged rows without rewriting their tag snapshots', () => {
    const items = stagedUploadItems([uploadFile('first.jpg')], 'primary', 123, ['source:one', 'person:alice']);
    const retargeted = retargetStagedUploadItems(items, 'archive');

    expect(retargeted[0].targetID).toBe('archive');
    expect(retargeted[0].tags).toEqual(['source:one', 'person:alice']);
  });

  it('edits one row in place without replacing the queue or row identity', () => {
    const items = stagedUploadItems([uploadFile('first.jpg'), uploadFile('second.jpg')], 'primary');
    const queueIdentity = items;
    const rowIdentity = items[1];

    setUploadItemTagsInPlace(items, 1, ['person:bob', ' ', 'person:bob', ' rating:safe ']);

    expect(items).toBe(queueIdentity);
    expect(items[1]).toBe(rowIdentity);
    expect(items[1].tags).toEqual(['person:bob', 'rating:safe']);
  });

  it('preserves a row tag snapshot when an import result replaces transport state', () => {
    const items = stagedUploadItems([uploadFile('first.jpg')], 'primary', 123, ['person:alice']);
    items[0].tagSyncBaseTags = ['person:alice'];
    const result = itemsFromResult({
      files: [{
        name: 'first.jpg',
        size: 10,
        target_id: 'primary',
        status: 'imported'
      }]
    } as never, items);

    expect(result[0].tags).toEqual(['person:alice']);
    expect(result[0].tagSyncBaseTags).toEqual(['person:alice']);
  });

  it('computes only the add/remove delta from the submitted baseline', () => {
    const item = queueItem(0);
    item.status = 'imported';
    item.tags = ['submitted', 'new'];
    item.tagSyncBaseTags = ['submitted', 'removed'];

    expect(uploadItemTagSyncDelta(item)).toEqual({ add: ['new'], remove: ['removed'] });
  });

  it('advances successful operations independently while preserving concurrent edits', () => {
    const items = [queueItem(0)];
    items[0].status = 'imported';
    items[0].tags = ['keep', 'add'];
    items[0].tagSyncBaseTags = ['keep', 'remove'];
    items[0].tagSyncPending = true;

    markUploadItemTagSyncAppliedInPlace(items, 0, 'add', ['add']);
    expect(items[0].tagSyncBaseTags).toEqual(['keep', 'remove', 'add']);
    expect(items[0].tagSyncPending).toBe(true);

    setUploadItemTagsInPlace(items, 0, ['keep', 'add', 'late']);
    expect(uploadItemTagSyncDelta(items[0])).toEqual({ add: ['late'], remove: ['remove'] });

    markUploadItemTagSyncAppliedInPlace(items, 0, 'remove', ['remove']);
    expect(items[0].tagSyncBaseTags).toEqual(['keep', 'add']);
    expect(items[0].tagSyncPending).toBe(true);
    expect(uploadItemTagSyncDelta(items[0])).toEqual({ add: ['late'], remove: [] });
  });

  it('rebases duplicate-existing rows onto full remote tags so pre-existing tags can be removed', () => {
    const items = [queueItem(0)];
    items[0].status = 'duplicate_existing';
    items[0].tags = ['submitted'];
    items[0].tagSyncBaseTags = ['submitted'];

    rebaseUploadItemTagsFromRemoteInPlace(items, 0, ['remote:existing', 'submitted']);
    expect(items[0].tags).toEqual(['remote:existing', 'submitted']);
    expect(items[0].tagSyncBaseTags).toEqual(['remote:existing', 'submitted']);
    expect(items[0].tagSyncPending).toBe(false);

    setUploadItemTagsInPlace(items, 0, ['submitted']);
    expect(uploadItemTagSyncDelta(items[0])).toEqual({ add: [], remove: ['remote:existing'] });
    expect(items[0].tagSyncPending).toBe(true);
  });

  it('preserves outstanding local edits while learning the authoritative remote tag baseline', () => {
    const items = [queueItem(0)];
    items[0].status = 'imported';
    items[0].tags = ['submitted', 'local:add'];
    items[0].tagSyncBaseTags = ['submitted', 'local:remove'];
    items[0].tagSyncPending = true;

    rebaseUploadItemTagsFromRemoteInPlace(items, 0, ['remote:existing', 'submitted', 'local:remove']);
    expect(items[0].tagSyncBaseTags).toEqual(['remote:existing', 'submitted', 'local:remove']);
    expect(items[0].tags).toEqual(['remote:existing', 'submitted', 'local:add']);
    expect(uploadItemTagSyncDelta(items[0])).toEqual({ add: ['local:add'], remove: ['local:remove'] });
    expect(items[0].tagSyncPending).toBe(true);
  });

  it('retains a successful partial baseline when a later operation fails', () => {
    const items = [queueItem(0)];
    items[0].status = 'imported';
    items[0].tags = ['keep', 'add'];
    items[0].tagSyncBaseTags = ['keep', 'remove'];
    items[0].tagSyncPending = true;

    markUploadItemTagSyncAppliedInPlace(items, 0, 'add', ['add']);
    markUploadItemTagSyncErrorInPlace(items, 0, 'remove failed');
    expect(items[0].tagSyncBaseTags).toEqual(['keep', 'remove', 'add']);
    expect(items[0].tagSyncPending).toBe(false);
    expect(items[0].tagSyncError).toBe('remove failed');

    setUploadItemTagsInPlace(items, 0, ['keep', 'add']);
    expect(items[0].tagSyncError).toBe('');
    expect(items[0].tagSyncPending).toBe(true);
    expect(uploadItemTagSyncDelta(items[0])).toEqual({ add: [], remove: ['remove'] });
  });
});

describe('upload import progress', () => {
  it('keeps durably admitted pending work queued until a worker starts it', () => {
    const items = [queueItem(0), queueItem(1)].map((item) => ({ ...item, status: 'queued' as const, progress: 100 }));
    const job = {
      id: 'upload-one',
      type: 'upload_import',
      status: 'pending',
      progress_total: 2,
      progress_completed: 0,
      progress_failed: 0,
      submitted_at: '2026-09-11T00:00:00Z'
    } as Job;

    expect(itemsFromJob(items, job).map((item) => [item.status, item.progress])).toEqual([
      ['queued', 100],
      ['queued', 100]
    ]);
  });

  it('marks only the completed file prefix at 100 percent', () => {
    const items = [queueItem(0), queueItem(1), queueItem(2)];
    const job = {
      id: 'upload-one',
      type: 'upload_import',
      status: 'running',
      progress: 2 / 3,
      progress_total: 3,
      progress_completed: 2,
      progress_completed_prefix: 2,
      progress_failed: 0,
      submitted_at: '2026-09-11T00:00:00Z'
    } as Job;

    expect(itemsFromJob(items, job).map((item) => [item.status, item.progress])).toEqual([
      ['importing', 100],
      ['importing', 100],
      ['importing', 0]
    ]);
  });

  it('does not guess row identity from aggregate completion alone', () => {
    const items = [queueItem(0), queueItem(1), queueItem(2)];
    const job = {
      id: 'upload-one',
      type: 'upload_import',
      status: 'running',
      progress: 2 / 3,
      progress_total: 3,
      progress_completed: 2,
      progress_failed: 0,
      submitted_at: '2026-09-11T00:00:00Z'
    } as Job;

    expect(itemsFromJob(items, job).map((item) => item.progress)).toEqual([0, 0, 0]);
  });

  it('keeps completed transport bars full while durable import progress starts at zero', () => {
    const items = [queueItem(0), queueItem(1), queueItem(2)].map((item) => ({ ...item, status: 'queued' as const, progress: 100 }));
    const job = {
      id: 'upload-one',
      type: 'upload_import',
      status: 'running',
      progress: 0,
      progress_total: 3,
      progress_completed: 0,
      progress_completed_prefix: 0,
      progress_failed: 0,
      submitted_at: '2026-09-11T00:00:00Z'
    } as Job;

    expect(itemsFromJob(items, job).map((item) => [item.status, item.progress])).toEqual([
      ['importing', 100],
      ['importing', 100],
      ['importing', 100]
    ]);
  });
});

describe('large upload queue updates', () => {
  it('keeps the 10k queue and row identities stable while progress and status counts update incrementally', () => {
    const items = Array.from({ length: 10_000 }, (_, index) => queueItem(index));
    const queueIdentity = items;
    const activeItems = items.slice(0, 4);
    const untouchedItem = items[9_999];
    const counts = countUploadStatuses(items);

    for (let index = 0; index < 4; index += 1) {
      replaceUploadItemInPlace(items, index, { ...items[index], status: 'uploading' }, counts);
    }

    for (let progress = 1; progress <= 100; progress += 1) {
      for (let index = 0; index < 4; index += 1) {
        const next = uploadProgressItem([items[index]], 0, progress)[0];
        replaceUploadItemInPlace(items, index, next, counts);
      }
    }

    expect(items).toBe(queueIdentity);
    expect(items.slice(0, 4)).toEqual(activeItems);
    for (let index = 0; index < 4; index += 1) expect(items[index]).toBe(activeItems[index]);
    expect(items[9_999]).toBe(untouchedItem);
    expect(items.slice(0, 4).map((item) => item.progress)).toEqual([100, 100, 100, 100]);
    expect(counts).toEqual({ waiting: 9_996, uploading: 4 });
    expect(uploadSummaryFromCounts(counts)).toBe('9996 waiting / 4 uploading');

    replaceUploadItemInPlace(items, 0, { ...items[0], status: 'imported', progress: 100 }, counts);
    expect(items[0]).toBe(activeItems[0]);
    expect(counts).toEqual({ waiting: 9_996, uploading: 3, imported: 1 });
    expect(uploadSummaryFromCounts(counts)).toBe('9996 waiting / 3 uploading / 1 imported');
  });
});
