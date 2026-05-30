import { ApiError } from '$lib/api/client';
import type { FileItem, Job } from '$lib/api/types';

export function formatBytes(size: number) {
  return new Intl.NumberFormat(undefined, {
    notation: size >= 1_000_000 ? 'compact' : 'standard',
    maximumFractionDigits: 1
  }).format(size);
}

export function errorMessage(error: unknown) {
  if (error instanceof ApiError) return error.message;
  if (error instanceof Error) return error.message;
  return 'The library could not be loaded.';
}

export function parseTags(value: string) {
  return value
    .split(/[\s,]+/)
    .map((tag) => tag.trim())
    .filter(Boolean);
}

export function selectedKind(files: FileItem[]) {
  const counts = files.reduce<Record<string, number>>((acc, file) => {
    acc[file.media_kind] = (acc[file.media_kind] ?? 0) + 1;
    return acc;
  }, {});
  return Object.entries(counts)
    .sort((a, b) => b[1] - a[1])
    .map(([kind, count]) => `${kind} ${count}`)
    .join(' / ');
}

export function isTerminalJob(job: Job) {
  return job.status === 'completed' || job.status === 'failed' || job.status === 'canceled';
}

export function jobStatusText(job: Job) {
  if (job.status === 'pending') return 'Queued';
  if (job.status === 'running') return 'Importing';
  if (job.status === 'completed') return 'Completed';
  if (job.status === 'canceled') return 'Canceled';
  return job.error ?? 'Upload failed';
}
