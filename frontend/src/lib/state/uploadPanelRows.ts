import { uploadSummaryFromCounts, type UploadItem, type UploadStatusCounts } from './uploadItems';

export type IndexedUploadRow = { item: UploadItem; index: number };
export type UploadQueueBatchSummary = { progress: number; counts: UploadStatusCounts; status: string };
export type UploadQueueBatch = { batchID?: number; rows: IndexedUploadRow[]; bytes: number; summary: UploadQueueBatchSummary };
export type UploadRowPage = { page: number; pageCount: number; rows: IndexedUploadRow[] };

type UploadQueueBatchSummaryState = {
  transportActive: boolean;
  itemProgressTotal: number;
  operationProgressMinimum: number;
  operationProgressCount: number;
};

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
  const summaryStates = new Map<number | undefined, UploadQueueBatchSummaryState>();
  for (const row of rows) {
    const batchID = row.item.batchID;
    let batch = batches.get(batchID);
    let summaryState = summaryStates.get(batchID);
    if (!batch || !summaryState) {
      batch = { batchID, rows: [], bytes: 0, summary: { progress: 0, counts: {}, status: '' } };
      summaryState = {
        transportActive: false,
        itemProgressTotal: 0,
        operationProgressMinimum: 100,
        operationProgressCount: 0
      };
      batches.set(batchID, batch);
      summaryStates.set(batchID, summaryState);
    }
    batch.rows.push(row);
    batch.bytes += row.item.size;

    const item = row.item;
    const counts = batch.summary.counts;
    counts[item.status] = (counts[item.status] ?? 0) + 1;
    if (item.status === 'waiting' || item.status === 'uploading') summaryState.transportActive = true;
    summaryState.itemProgressTotal += Math.max(0, Math.min(100, item.progress));
    if (typeof item.operationProgress === 'number') {
      summaryState.operationProgressMinimum = Math.min(
        summaryState.operationProgressMinimum,
        Math.max(0, Math.min(100, item.operationProgress))
      );
      summaryState.operationProgressCount += 1;
    }
  }

  const grouped = [...batches.values()];
  for (const batch of grouped) {
    const summaryState = summaryStates.get(batch.batchID)!;
    batch.summary.progress = !summaryState.transportActive && summaryState.operationProgressCount > 0
      ? Math.round(summaryState.operationProgressMinimum)
      : Math.round(summaryState.itemProgressTotal / batch.rows.length);
    batch.summary.status = uploadSummaryFromCounts(batch.summary.counts);
  }
  return grouped.sort((left, right) => {
    if (left.batchID == null) return right.batchID == null ? 0 : 1;
    if (right.batchID == null) return -1;
    return right.batchID - left.batchID;
  });
}

// Batch summaries are computed in the same pass that groups queue rows. Keeping
// this accessor preserves the component/test call site while avoiding a second
// O(batch size) scan on every reactive queue update.
export function summarizeUploadQueueBatch(batch: UploadQueueBatch): UploadQueueBatchSummary {
  return batch.summary;
}

export function paginateUploadRows(rows: IndexedUploadRow[], requestedPage: number, requestedPageSize: number): UploadRowPage {
  const pageSize = Math.max(1, Math.trunc(requestedPageSize));
  const pageCount = Math.max(1, Math.ceil(rows.length / pageSize));
  const page = Math.min(Math.max(0, Math.trunc(requestedPage)), pageCount - 1);
  const start = page * pageSize;
  return { page, pageCount, rows: rows.slice(start, start + pageSize) };
}
