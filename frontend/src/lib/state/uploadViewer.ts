import type { UploadItem } from './uploadItems';

export type UploadViewerScope = 'staged' | 'queue';

export function uploadViewerScope(item: UploadItem): UploadViewerScope {
  return item.status === 'staged' ? 'staged' : 'queue';
}

export function uploadViewerIndices(items: UploadItem[], scope: UploadViewerScope): number[] {
  const indices: number[] = [];
  for (let index = 0; index < items.length; index += 1) {
    const item = items[index];
    if (item && uploadViewerScope(item) === scope) indices.push(index);
  }
  return indices;
}

export function uploadItemHasLocalViewer(item: UploadItem): boolean {
  if (!item.previewFile) return false;
  const mediaType = (item.previewFile.type || item.type).trim().toLowerCase();
  return mediaType.startsWith('image/') || mediaType.startsWith('video/') || mediaType.startsWith('audio/');
}

export function uploadItemCanOpenViewer(item: UploadItem): boolean {
  return Boolean(item.remoteFileID) || uploadItemHasLocalViewer(item);
}

export function uploadViewerNavigableIndices(items: UploadItem[], scope: UploadViewerScope): number[] {
  return uploadViewerIndices(items, scope).filter((index) => {
    const item = items[index];
    return Boolean(item && uploadItemCanOpenViewer(item));
  });
}

export function uploadViewerNeighborIndex(
  items: UploadItem[],
  activeIndex: number,
  scope: UploadViewerScope,
  delta: number
): number | null {
  const indices = uploadViewerNavigableIndices(items, scope);
  if (!indices.length) return null;
  const position = indices.indexOf(activeIndex);
  if (position < 0) return null;
  return indices[(position + delta + indices.length) % indices.length] ?? null;
}
