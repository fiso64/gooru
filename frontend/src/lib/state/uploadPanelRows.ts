import type { UploadItem } from './uploadItems';

export type IndexedUploadRow = { item: UploadItem; index: number };
export type UploadQueueBatch = { batchID?: number; rows: IndexedUploadRow[]; bytes: number };
export type UploadRowPage = { page: number; pageCount: number; rows: IndexedUploadRow[] };

export function partitionUploadRows(rows: IndexedUploadRow[]): { staged: IndexedUploadRow[]; queue: IndexedUploadRow[] } {
  const staged: IndexedUploadRow[] = [];
  const queue: IndexedUploadRow[] = [];
  for (const row of rows) {
    if (row.item.status === 'staged') staged.push(row);
    else queue.push(row);
  }
  return { staged, queue };
}

export function filterUploadRows(rows: IndexedUploadRow[], query: string): IndexedUploadRow[] {
  const normalizedQuery = query.trim().toLowerCase();
  if (!normalizedQuery) return rows;
  return rows.filter((row) => row.item.name.toLowerCase().includes(normalizedQuery));
}

export function groupUploadQueueRows(rows: IndexedUploadRow[]): UploadQueueBatch[] {
  const batches = new Map<number | undefined, UploadQueueBatch>();
  for (const row of rows) {
    const batchID = row.item.batchID;
    let batch = batches.get(batchID);
    if (!batch) {
      batch = { batchID, rows: [], bytes: 0 };
      batches.set(batchID, batch);
    }
    batch.rows.push(row);
    batch.bytes += row.item.size;
  }
  return [...batches.values()].sort((left, right) => {
    if (left.batchID == null) return right.batchID == null ? 0 : 1;
    if (right.batchID == null) return -1;
    return right.batchID - left.batchID;
  });
}

export function paginateUploadRows(rows: IndexedUploadRow[], requestedPage: number, requestedPageSize: number): UploadRowPage {
  const pageSize = Math.max(1, Math.trunc(requestedPageSize));
  const pageCount = Math.max(1, Math.ceil(rows.length / pageSize));
  const page = Math.min(Math.max(0, Math.trunc(requestedPage)), pageCount - 1);
  const start = page * pageSize;
  return { page, pageCount, rows: rows.slice(start, start + pageSize) };
}
