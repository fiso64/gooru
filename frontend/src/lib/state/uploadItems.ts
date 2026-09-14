import type { Job, UploadImportResponse } from '$lib/api/types';
import { isTerminalJob } from '$lib/utils/format';

export type UploadItemStatus =
  | 'staged'
  | 'waiting'
  | 'uploading'
  | 'queued'
  | 'importing'
  | 'imported'
  | 'uploaded'
  | 'duplicate_existing'
  | 'duplicate_in_batch'
  | 'skipped'
  | 'error'
  | 'canceled';

export type UploadAddedAtStrategy = 'queue' | 'reverse_queue' | 'modtime';

export interface UploadItem {
  name: string;
  size: number;
  type: string;
  previewFile?: File;
  batchID?: number;
  tags?: string[];
  targetID?: string;
  queueTimeMs?: number;
  remoteFileID?: string;
  status: UploadItemStatus;
  progress: number;
  error?: string;
}

export type UploadTargetOption = { id: string; name: string; added_at_strategy?: UploadAddedAtStrategy; default_tags?: string[] };
export type UploadStatusCounts = Partial<Record<UploadItemStatus, number>>;

type UploadResultFileWithIdentity = UploadImportResponse['files'][number] & { id?: string };

export function effectiveUploadTargetID(targetID: string, targets: UploadTargetOption[]): string {
  if (targetID && targets.some((target) => target.id === targetID)) return targetID;
  return targets[0]?.id ?? '';
}

export function normalizeUploadItemTags(tags: string[]): string[] {
  const result: string[] = [];
  const seen = new Set<string>();
  for (const value of tags) {
    const tag = value.trim();
    if (!tag || seen.has(tag)) continue;
    seen.add(tag);
    result.push(tag);
  }
  return result;
}

export function stagedUploadItems(files: File[], targetID = '', queueTimeMs = Date.now(), tags: string[] = []): UploadItem[] {
  const initialTags = normalizeUploadItemTags(tags);
  return files.map((file) => ({
    name: file.name,
    size: file.size,
    type: file.type,
    previewFile: file,
    tags: [...initialTags],
    targetID,
    queueTimeMs,
    status: 'staged',
    progress: 0
  }));
}

export function retargetStagedUploadItems(items: UploadItem[], targetID: string): UploadItem[] {
  return items.map((item) => item.status === 'staged' ? { ...item, targetID } : item);
}

export function setUploadItemTagsInPlace(items: UploadItem[], index: number, tags: string[]): void {
  const current = items[index];
  if (!current) return;
  current.tags = normalizeUploadItemTags(tags);
}

export function waitingUploadItems(items: UploadItem[]): UploadItem[] {
  return items.map((item) => ({ ...item, status: 'waiting', progress: 0, error: '' }));
}

export function uploadingItem(items: UploadItem[], index: number): UploadItem[] {
  return updateUploadItem(items, index, (item) => ({ ...item, status: 'uploading', progress: 0, error: '' }));
}

export function uploadProgressItem(items: UploadItem[], index: number, progress: number): UploadItem[] {
  const safeProgress = Math.max(0, Math.min(100, Math.round(progress)));
  return updateUploadItem(items, index, (item) => ({ ...item, status: 'uploading', progress: safeProgress }));
}

export function queuedItem(items: UploadItem[], index: number): UploadItem[] {
  return updateUploadItem(items, index, (item) => ({ ...item, status: 'queued', progress: 100 }));
}

export function itemFromJob(items: UploadItem[], index: number, job: Job): UploadItem[] {
  const current = items[index];
  if (!current) return items;
  const next = itemsFromJob([current], job)[0];
  return next ? updateUploadItem(items, index, () => next) : items;
}

export function itemFromResult(items: UploadItem[], index: number, response: UploadImportResponse): UploadItem[] {
  const current = items[index];
  if (!current) return items;
  const next = itemsFromResult(response, [current])[0];
  return next ? updateUploadItem(items, index, () => next) : items;
}

export function itemsFromJob(items: UploadItem[], job: Job): UploadItem[] {
  const terminal = isTerminalJob(job);
  const completedPrefix = Math.max(0, Math.min(items.length, Math.trunc(job.progress_completed_prefix ?? 0)));
  const status: UploadItemStatus =
    job.status === 'completed' ? 'imported' :
    job.status === 'canceled' ? 'canceled' :
    job.status === 'failed' ? 'error' :
    job.status === 'pending' ? 'queued' : 'importing';
  if (job.status === 'completed' && isUploadImportResponse(job.result)) {
    return itemsFromResult(job.result, items);
  }
  return items.map((item, index) => ({
    ...item,
    status,
    progress: terminal || index < completedPrefix ? 100 : item.progress,
    error: job.status === 'failed' ? job.error ?? 'Import failed' : item.error
  }));
}

export function itemsFromResult(response: UploadImportResponse, previous: UploadItem[] = []): UploadItem[] {
  return response.files.map((file, index) => {
    const prior = previous.find((item) => item.name === file.name) ?? previous[index];
    return {
      name: file.name,
      size: file.size,
      type: prior?.type ?? '',
      previewFile: prior?.previewFile,
      batchID: prior?.batchID,
      tags: prior?.tags ? [...prior.tags] : [],
      targetID: file.target_id,
      queueTimeMs: prior?.queueTimeMs,
      remoteFileID: (file as UploadResultFileWithIdentity).id ?? prior?.remoteFileID,
      status: file.status,
      progress: 100,
      error: file.error
    };
  });
}

export function countUploadStatuses(items: UploadItem[]): UploadStatusCounts {
  const counts: UploadStatusCounts = {};
  for (const item of items) counts[item.status] = (counts[item.status] ?? 0) + 1;
  return counts;
}

export function transitionUploadStatus(counts: UploadStatusCounts, previous: UploadItemStatus, next: UploadItemStatus): void {
  if (previous === next) return;
  const previousCount = counts[previous] ?? 0;
  if (previousCount <= 1) delete counts[previous];
  else counts[previous] = previousCount - 1;
  counts[next] = (counts[next] ?? 0) + 1;
}

export function replaceUploadItemInPlace(
  items: UploadItem[],
  index: number,
  next: UploadItem | undefined,
  counts: UploadStatusCounts
): void {
  const current = items[index];
  if (!current || !next) return;
  const previousStatus = current.status;
  Object.assign(current, next);
  transitionUploadStatus(counts, previousStatus, next.status);
}

export function uploadSummaryFromCounts(counts: UploadStatusCounts): string {
  return Object.entries(counts)
    .filter(([, count]) => Boolean(count))
    .map(([status, count]) => `${count} ${status.replace(/_/g, ' ')}`)
    .join(' / ');
}

export function uploadSummary(items: UploadItem[]): string {
  if (!items.length) return '';
  return uploadSummaryFromCounts(countUploadStatuses(items));
}

function updateUploadItem(items: UploadItem[], index: number, update: (item: UploadItem) => UploadItem): UploadItem[] {
  return items.map((item, itemIndex) => (itemIndex === index ? update(item) : item));
}

function isUploadImportResponse(value: unknown): value is UploadImportResponse {
  return Boolean(value && typeof value === 'object' && Array.isArray((value as UploadImportResponse).files));
}
