import { countUploadStatuses, uploadSummaryFromCounts, type UploadItem, type UploadStatusCounts } from './uploadItems';

export type IndexedUploadRow = { item: UploadItem; index: number };
export type UploadQueueBatch = { batchID?: number; rows: IndexedUploadRow[]; bytes: number };
export type UploadQueueBatchSummary = { progress: number; counts: UploadStatusCounts; status: string };
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

export function summarizeUploadQueueBatch(batch: UploadQueueBatch): UploadQueueBatchSummary {
  const items = batch.rows.map((row) => row.item);
  const counts = countUploadStatuses(items);
  if (!items.length) return { progress: 0, counts, status: '' };

  const transportActive = items.some((item) => item.status === 'waiting' || item.status === 'uploading');
  const operationProgress = transportActive
    ? []
    : items.flatMap((item) => typeof item.operationProgress === 'number'
      ? [Math.max(0, Math.min(100, item.operationProgress))]
      : []);
  const progress = operationProgress.length
    ? Math.round(Math.min(...operationProgress))
    : Math.round(items.reduce((sum, item) => sum + Math.max(0, Math.min(100, item.progress)), 0) / items.length);
  return { progress, counts, status: uploadSummaryFromCounts(counts) };
}

export function paginateUploadRows(rows: IndexedUploadRow[], requestedPage: number, requestedPageSize: number): UploadRowPage {
  const pageSize = Math.max(1, Math.trunc(requestedPageSize));
  const pageCount = Math.max(1, Math.ceil(rows.length / pageSize));
  const page = Math.min(Math.max(0, Math.trunc(requestedPage)), pageCount - 1);
  const start = page * pageSize;
  return { page, pageCount, rows: rows.slice(start, start + pageSize) };
}
