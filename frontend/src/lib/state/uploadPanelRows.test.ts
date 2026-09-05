import { describe, expect, it } from 'vitest';
import { partitionUploadRows, type IndexedUploadRow } from './uploadPanelRows';

describe('partitionUploadRows', () => {
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
});
