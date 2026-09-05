import type { UploadItem } from './uploadItems';

export type IndexedUploadRow = { item: UploadItem; index: number };

export function partitionUploadRows(rows: IndexedUploadRow[]): { staged: IndexedUploadRow[]; queue: IndexedUploadRow[] } {
  const staged: IndexedUploadRow[] = [];
  const queue: IndexedUploadRow[] = [];
  for (const row of rows) {
    if (row.item.status === 'staged') staged.push(row);
    else queue.push(row);
  }
  return { staged, queue };
}
