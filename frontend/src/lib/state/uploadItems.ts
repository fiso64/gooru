import type { Job, UploadImportResponse } from '$lib/api/types';
import { isTerminalJob } from '$lib/utils/format';

export type UploadItemStatus =
  | 'staged'
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

export interface UploadItem {
  name: string;
  size: number;
  type: string;
  targetID?: string;
  status: UploadItemStatus;
  progress: number;
  error?: string;
}

export type UploadTargetOption = { id: string; name: string };

export function effectiveUploadTargetID(targetID: string, targets: UploadTargetOption[]): string {
  if (targetID && targets.some((target) => target.id === targetID)) return targetID;
  return targets[0]?.id ?? '';
}

export function stagedUploadItems(files: File[], targetID = ''): UploadItem[] {
  return files.map((file) => ({
    name: file.name,
    size: file.size,
    type: file.type,
    targetID,
    status: 'staged',
    progress: 0
  }));
}

export function uploadingItems(items: UploadItem[]): UploadItem[] {
  return items.map((item) => ({ ...item, status: 'uploading', progress: Math.max(item.progress, 12), error: '' }));
}

export function queuedItems(items: UploadItem[]): UploadItem[] {
  return items.map((item) => ({ ...item, status: 'queued', progress: Math.max(item.progress, 20) }));
}

export function itemsFromJob(items: UploadItem[], job: Job): UploadItem[] {
  const progress = Math.round((job.progress ?? (isTerminalJob(job) ? 1 : 0.5)) * 100);
  const status: UploadItemStatus =
    job.status === 'completed' ? 'imported' : job.status === 'canceled' ? 'canceled' : job.status === 'failed' ? 'error' : 'importing';
  if (job.status === 'completed' && isUploadImportResponse(job.result)) {
    return itemsFromResult(job.result, items);
  }
  return items.map((item) => ({
    ...item,
    status,
    progress,
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
      targetID: file.target_id,
      status: file.status,
      progress: 100,
      error: file.error
    };
  });
}

export function uploadSummary(items: UploadItem[]): string {
  if (!items.length) return '';
  const counts = items.reduce<Record<string, number>>((acc, item) => {
    acc[item.status] = (acc[item.status] ?? 0) + 1;
    return acc;
  }, {});
  return Object.entries(counts)
    .map(([status, count]) => `${count} ${status.replace(/_/g, ' ')}`)
    .join(' / ');
}

function isUploadImportResponse(value: unknown): value is UploadImportResponse {
  return Boolean(value && typeof value === 'object' && Array.isArray((value as UploadImportResponse).files));
}
