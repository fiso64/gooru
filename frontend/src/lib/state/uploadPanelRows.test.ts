import { describe, expect, it } from 'vitest';
import { filterUploadRows, groupUploadQueueRows, paginateUploadRows, partitionUploadRows, type IndexedUploadRow } from './uploadPanelRows';

function uploadRow(index: number, batchID?: number): IndexedUploadRow {
  return {
    index,
    item: {
      name: `file-${index}.jpg`,
      size: 1024 + index,
      type: 'image/jpeg',
      batchID,
      status: 'waiting',
      progress: 0
    }
  };
}

describe('upload panel rows', () => {
  it('partitions a 10k upload queue in one pass while preserving row identity and index', () => {
    const rows: IndexedUploadRow[] = Array.from({ length: 10_000 }, (_, index) => ({
      index,
      item: {
        name: `file-${index}.jpg`,
        size: 1024,
        type: 'image/jpeg',
        status: index < 250 ? 'staged' : index % 3 === 0 ? 'imported' : 'waiting',
        progress: index % 3 === 0 ? 100 : 0
      }
    }));

    const result = partitionUploadRows(rows);

    expect(result.staged).toHaveLength(250);
    expect(result.queue).toHaveLength(9_750);
    expect(result.staged[249]).toBe(rows[249]);
    expect(result.queue[0]).toBe(rows[250]);
    expect(result.queue.at(-1)).toBe(rows[9_999]);
    expect(result.queue.at(-1)?.index).toBe(9_999);
  });

  it('filters staged rows by filename without changing the submitted row collection', () => {
    const rows = [uploadRow(0), uploadRow(1), uploadRow(2)];
    rows[1]!.item.name = 'Holiday Sunset.JPG';

    expect(filterUploadRows(rows, '')).toBe(rows);
    expect(filterUploadRows(rows, '  sunset ')).toEqual([rows[1]]);
    expect(filterUploadRows(rows, 'SUNSET')).toEqual([rows[1]]);
    expect(rows).toHaveLength(3);
  });

  it('groups queue rows by batch newest-first while preserving row order and byte totals', () => {
    const earlier = uploadRow(0, 1);
    const newerA = uploadRow(1, 2);
    const newerB = uploadRow(2, 2);
    const legacy = uploadRow(3);
    const batches = groupUploadQueueRows([earlier, newerA, newerB, legacy]);

    expect(batches.map((batch) => batch.batchID)).toEqual([2, 1, undefined]);
    expect(batches[0]?.rows).toEqual([newerA, newerB]);
    expect(batches[0]?.bytes).toBe(newerA.item.size + newerB.item.size);
    expect(batches[1]?.rows).toEqual([earlier]);
    expect(batches[2]?.rows).toEqual([legacy]);
  });

  it('paginates each batch independently and clamps stale page indices', () => {
    const rows = Array.from({ length: 205 }, (_, index) => uploadRow(index, 7));

    const last = paginateUploadRows(rows, 2, 100);
    expect(last.pageCount).toBe(3);
    expect(last.page).toBe(2);
    expect(last.rows).toHaveLength(5);
    expect(last.rows[0]?.index).toBe(200);

    expect(paginateUploadRows(rows, 99, 100).page).toBe(2);
    expect(paginateUploadRows(rows, -5, 100).page).toBe(0);
  });
});
