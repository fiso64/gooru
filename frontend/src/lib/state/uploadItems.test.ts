import { describe, expect, it } from 'vitest';
import type { Job } from '$lib/api/types';
import {
  countUploadStatuses,
  effectiveUploadTargetID,
  itemsFromJob,
  replaceUploadItemInPlace,
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

describe('upload import progress', () => {
  it('marks only the completed file prefix at 100 percent', () => {
    const items = [queueItem(0), queueItem(1), queueItem(2)];
    const job = {
      id: 'upload-one',
      type: 'upload_import',
      kind: 'upload_import',
      status: 'running',
      progress: 2 / 3,
      progress_total: 3,
      progress_completed: 2,
      progress_failed: 0,
      submitted_at: '2026-09-11T00:00:00Z',
      created_at: '2026-09-11T00:00:00Z'
    } as Job;

    expect(itemsFromJob(items, job).map((item) => [item.status, item.progress])).toEqual([
      ['importing', 100],
      ['importing', 100],
      ['importing', 0]
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
