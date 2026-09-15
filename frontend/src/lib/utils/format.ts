import { ApiError } from '$lib/api/client';
import type { FileItem, Job } from '$lib/api/types';

export function formatBytes(size: number) {
  if (!Number.isFinite(size) || size <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.min(units.length - 1, Math.floor(Math.log(size) / Math.log(1024)));
  const value = size / 1024 ** index;
  return `${value.toFixed(value >= 10 || index === 0 ? 0 : 1)} ${units[index]}`;
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

export function mediaDimensions(file: FileItem) {
  const width = file.metadata.image_width ?? file.metadata.video_width;
  const height = file.metadata.image_height ?? file.metadata.video_height;
  return width && height ? `${width}x${height}` : '';
}

export function mediaDuration(file: { metadata?: { video_duration?: number; audio_duration?: number } }) {
  const seconds = file.metadata?.video_duration ?? file.metadata?.audio_duration;
  if (!seconds) return '';
  const mins = Math.floor(seconds / 60);
  const secs = Math.round(seconds % 60).toString().padStart(2, '0');
  return `${mins}:${secs}`;
}

export function groupTags(tags: string[]) {
  const groups = new Map<string, string[]>();
  const other: string[] = [];
  for (const tag of tags) {
    const index = tag.indexOf(':');
    const ns = index > 0 ? tag.slice(0, index) : '';
    if (!ns) {
      other.push(tag);
      continue;
    }
    const list = groups.get(ns) ?? [];
    list.push(tag);
    groups.set(ns, list);
  }
  const result = Array.from(groups.entries()).map(([namespace, items]) => ({ namespace, tags: items }));
  if (other.length) result.push({ namespace: '', tags: other });
  return result;
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
