import type { FileItem } from '$lib/api/types';

export interface VirtualGrid {
  files: FileItem[];
  totalHeight: number;
  offsetTop: number;
  columns: number;
  rowHeight: number;
  needsPrevious: boolean;
  needsNext: boolean;
}

const gridPadding = 16 * 2;
const gridGap = 5;
const minCardWidth = 180;

export function gridColumns(containerWidth: number) {
  const innerWidth = Math.max(0, containerWidth - gridPadding);
  return Math.max(1, Math.floor((innerWidth + gridGap) / (minCardWidth + gridGap)));
}

export function gridRowHeight(containerWidth: number, columns = gridColumns(containerWidth)) {
  const innerWidth = Math.max(0, containerWidth - gridPadding);
  const cardWidth = columns > 0 ? (innerWidth - gridGap * (columns - 1)) / columns : minCardWidth;
  return Math.max(minCardWidth, cardWidth) + gridGap;
}

export function virtualGrid(
  files: FileItem[],
  containerWidth: number,
  viewportHeight: number,
  scrollY: number,
  gridTop: number,
  totalItems = files.length,
  retainedStartIndex = 0
): VirtualGrid {
  const columns = gridColumns(containerWidth);
  const rowHeight = gridRowHeight(containerWidth, columns);
  const overscanRows = 4;
  const totalRows = Math.ceil(Math.max(totalItems, retainedStartIndex + files.length) / columns);
  const viewportStart = Math.max(0, scrollY - gridTop);
  const startRow = Math.max(0, Math.floor(viewportStart / rowHeight) - overscanRows);
  const visibleRows = Math.ceil(viewportHeight / rowHeight) + overscanRows * 2;
  const endRow = Math.min(totalRows, startRow + visibleRows);
  const retainedEndIndex = retainedStartIndex + files.length;
  const globalStartIndex = startRow * columns;
  const globalEndIndex = endRow * columns;
  const startIndex = Math.max(retainedStartIndex, globalStartIndex);
  const endIndex = Math.min(retainedEndIndex, globalEndIndex);
  return {
    files: startIndex < endIndex ? files.slice(startIndex - retainedStartIndex, endIndex - retainedStartIndex) : [],
    totalHeight: totalRows * rowHeight,
    offsetTop: Math.floor(startIndex / columns) * rowHeight,
    columns,
    rowHeight,
    needsPrevious: globalStartIndex < retainedStartIndex,
    needsNext: globalEndIndex > retainedEndIndex
  };
}
