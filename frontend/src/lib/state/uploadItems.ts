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
  tagSyncPending?: boolean;
  tagSyncError?: string;
  targetID?: string;
  queueTimeMs?: number;
  remoteFileID?: string;
  operationProgress?: number;
  status: UploadItemStatus;
  progress: number;
  error?: string;
}

export type UploadTargetOption = { id: string; name: string; added_at_strategy?: UploadAddedAtStrategy; default_tags?: string[] };
export type UploadStatusCounts = Partial<Record<UploadItemStatus, number>>;

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

function sameUploadItemTags(left: string[] = [], right: string[] = []): boolean {
  return left.length === right.length && left.every((tag, index) => tag === right[index]);
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
  current.tagSyncError = '';
  current.tagSyncPending = current.status !== 'staged';
}

export function markUploadItemTagsSyncedInPlace(items: UploadItem[], index: number, syncedTags: string[]): void {
  const current = items[index];
  if (!current) return;
  const normalized = normalizeUploadItemTags(syncedTags);
  current.tagSyncPending = !sameUploadItemTags(current.tags ?? [], normalized);
  current.tagSyncError = '';
}

export function markUploadItemTagSyncErrorInPlace(items: UploadItem[], index: number, message: string): void {
  const current = items[index];
  if (!current) return;
  current.tagSyncPending = false;
  current.tagSyncError = message;
}

export function waitingUploadItems(items: UploadItem[]): UploadItem[] {
  return items.map((item) => ({ ...item, status: 'waiting', progress: 0, operationProgress: undefined, error: '' }));
}

export function uploadingItem(items: UploadItem[], index: number): UploadItem[] {
  return updateUploadItem(items, index, (item) => ({ ...item, status: 'uploading', progress: 0, operationProgress: undefined, error: '' }));
}

export function uploadProgressItem(items: UploadItem[], index: number, progress: number): UploadItem[] {
  const safeProgress = Math.max(0, Math.min(100, Math.round(progress)));
  return updateUploadItem(items, index, (item) => ({ ...item, status: 'uploading', progress: safeProgress }));
}

export function queuedItem(items: UploadItem[], index: number): UploadItem[] {
  return updateUploadItem(items, index, (item) => ({ ...item, status: 'queued', progress: 100, operationProgress: 0 }));
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
  const operationProgress = jobOperationProgress(job);
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
    operationProgress,
    error: job.status === 'failed' ? job.error ?? 'Import failed' : item.error
  }));
}

export function itemsFromResult(response: UploadImportResponse, previous: UploadItem[] = []): UploadItem[] {
  const previousIndicesByName = new Map<string, number[]>();
  for (let index = 0; index < previous.length; index += 1) {
    const item = previous[index];
    if (!item) continue;
    const matches = previousIndicesByName.get(item.name);
    if (matches) matches.push(index);
    else previousIndicesByName.set(item.name, [index]);
  }
  const previousNameCursor = new Map<string, number>();
  const consumedPrevious = new Set<number>();
  let fallbackCursor = 0;

  const consumePrevious = (name: string, responseIndex: number): UploadItem | undefined => {
    const matches = previousIndicesByName.get(name) ?? [];
    let cursor = previousNameCursor.get(name) ?? 0;
    while (cursor < matches.length && consumedPrevious.has(matches[cursor]!)) cursor += 1;
    previousNameCursor.set(name, cursor + 1);
    const namedIndex = matches[cursor];
    if (namedIndex !== undefined) {
      consumedPrevious.add(namedIndex);
      return previous[namedIndex];
    }

    if (previous[responseIndex] && !consumedPrevious.has(responseIndex)) {
      consumedPrevious.add(responseIndex);
      return previous[responseIndex];
    }
    while (fallbackCursor < previous.length && consumedPrevious.has(fallbackCursor)) fallbackCursor += 1;
    if (fallbackCursor >= previous.length) return undefined;
    const fallbackIndex = fallbackCursor;
    fallbackCursor += 1;
    consumedPrevious.add(fallbackIndex);
    return previous[fallbackIndex];
  };

  return response.files.map((file, index) => {
    const prior = consumePrevious(file.name, index);
    return {
      name: file.name,
      size: file.size,
      type: prior?.type ?? '',
      previewFile: prior?.previewFile,
      batchID: prior?.batchID,
      tags: prior?.tags ? [...prior.tags] : [],
      tagSyncPending: prior?.tagSyncPending,
      tagSyncError: prior?.tagSyncError,
      targetID: file.target_id,
      queueTimeMs: prior?.queueTimeMs,
      remoteFileID: file.id ?? prior?.remoteFileID,
      operationProgress: 100,
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

function jobOperationProgress(job: Job): number {
  if (isTerminalJob(job)) return 100;
  if (typeof job.progress === 'number' && Number.isFinite(job.progress)) {
    return Math.max(0, Math.min(100, Math.round(job.progress * 100)));
  }
  const total = Math.max(0, job.progress_total ?? 0);
  if (total <= 0) return 0;
  const completed = Math.max(0, job.progress_completed ?? 0);
  return Math.max(0, Math.min(100, Math.round((completed / total) * 100)));
}

function updateUploadItem(items: UploadItem[], index: number, update: (item: UploadItem) => UploadItem): UploadItem[] {
  return items.map((item, itemIndex) => (itemIndex === index ? update(item) : item));
}

function isUploadImportResponse(value: unknown): value is UploadImportResponse {
  return Boolean(value && typeof value === 'object' && Array.isArray((value as UploadImportResponse).files));
}
