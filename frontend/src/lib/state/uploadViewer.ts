import type { UploadItem } from './uploadItems';

export type UploadViewerScope = 'staged' | 'queue';

export function uploadViewerScope(item: UploadItem): UploadViewerScope {
  return item.status === 'staged' ? 'staged' : 'queue';
}

export function uploadItemHasLocalViewer(item: UploadItem): boolean {
  if (!item.previewFile) return false;
  const mediaType = (item.previewFile.type || item.type).trim().toLowerCase();
  return mediaType.startsWith('image/') || mediaType.startsWith('video/') || mediaType.startsWith('audio/');
}

export function uploadItemCanOpenViewer(item: UploadItem): boolean {
  return Boolean(item.remoteFileID) || uploadItemHasLocalViewer(item);
}

export function uploadViewerNeighborIndex(
  items: UploadItem[],
  activeIndex: number,
  scope: UploadViewerScope,
  delta: -1 | 1
): number | null {
  const activeItem = items[activeIndex];
  if (!activeItem || uploadViewerScope(activeItem) !== scope || !uploadItemCanOpenViewer(activeItem)) return null;

  for (let offset = 1; offset <= items.length; offset += 1) {
    const index = (activeIndex + delta * offset + items.length) % items.length;
    const item = items[index];
    if (item && uploadViewerScope(item) === scope && uploadItemCanOpenViewer(item)) return index;
  }
  return null;
}
