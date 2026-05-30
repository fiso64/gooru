import type { FileItem } from '$lib/api/types';

export interface VirtualGrid {
  files: FileItem[];
  totalHeight: number;
  offsetTop: number;
}

export function gridColumns(viewportWidth: number) {
  if (viewportWidth >= 1536) return 8;
  if (viewportWidth >= 1280) return 6;
  if (viewportWidth >= 1024) return 4;
  if (viewportWidth >= 640) return 3;
  return 2;
}

export function virtualGrid(files: FileItem[], viewportWidth: number, viewportHeight: number, scrollY: number): VirtualGrid {
  const columns = gridColumns(viewportWidth);
  const rowHeight = viewportWidth >= 1024 ? 432 : viewportWidth >= 640 ? 392 : 352;
  const overscanRows = 4;
  const totalRows = Math.ceil(files.length / columns);
  const startRow = Math.max(0, Math.floor((scrollY - 260) / rowHeight) - overscanRows);
  const visibleRows = Math.ceil(viewportHeight / rowHeight) + overscanRows * 2;
  const endRow = Math.min(totalRows, startRow + visibleRows);
  const startIndex = startRow * columns;
  const endIndex = Math.min(files.length, endRow * columns);
  return {
    files: files.slice(startIndex, endIndex),
    totalHeight: totalRows * rowHeight,
    offsetTop: startRow * rowHeight
  };
}
