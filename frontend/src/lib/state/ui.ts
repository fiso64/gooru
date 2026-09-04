import type { FileItem } from '$lib/api/types';
import { defaultGridSize } from '$lib/stores/runtimeConfig';

export interface VirtualGrid {
  files: FileItem[];
  totalHeight: number;
  offsetTop: number;
  columns: number;
  cardWidth: number;
  rowHeight: number;
  needsPrevious: boolean;
  needsNext: boolean;
}

const gridPadding = 16 * 2;
const gridGap = 5;
const overscanRows = 4;
const virtualWindowStrideRows = 3;

export function gridColumns(containerWidth: number, minCardWidth = defaultGridSize) {
  const innerWidth = Math.max(0, containerWidth - gridPadding);
  return Math.max(1, Math.floor((innerWidth + gridGap) / (minCardWidth + gridGap)));
}

export function gridCardWidth(containerWidth: number, minCardWidth = defaultGridSize, columns = gridColumns(containerWidth, minCardWidth)) {
  const innerWidth = Math.max(0, containerWidth - gridPadding);
  const cardWidth = columns > 0 ? (innerWidth - gridGap * (columns - 1)) / columns : minCardWidth;
  return Math.max(minCardWidth, cardWidth);
}

export function gridRowHeight(containerWidth: number, minCardWidth = defaultGridSize, columns = gridColumns(containerWidth, minCardWidth)) {
  return gridCardWidth(containerWidth, minCardWidth, columns) + gridGap;
}

export function virtualGridStartRow(scrollY: number, gridTop: number, rowHeight: number) {
  if (!Number.isFinite(rowHeight) || rowHeight <= 0) return 0;
  const viewportStart = Math.max(0, scrollY - gridTop);
  const overscannedStart = Math.max(0, Math.floor(viewportStart / rowHeight) - overscanRows);
  return Math.floor(overscannedStart / virtualWindowStrideRows) * virtualWindowStrideRows;
}

export function virtualGrid(
  files: FileItem[],
  containerWidth: number,
  viewportHeight: number,
  scrollY: number,
  gridTop: number,
  totalItems = files.length,
  retainedStartIndex = 0,
  minCardWidth = defaultGridSize
): VirtualGrid {
  const columns = gridColumns(containerWidth, minCardWidth);
  const cardWidth = gridCardWidth(containerWidth, minCardWidth, columns);
  const rowHeight = cardWidth + gridGap;
  const totalRows = Math.ceil(Math.max(totalItems, retainedStartIndex + files.length) / columns);
  const startRow = virtualGridStartRow(scrollY, gridTop, rowHeight);
  // Keep enough trailing rows for the window to stay mounted while its
  // chunked start lags the viewport by up to stride - 1 rows.
  const visibleRows = Math.ceil(viewportHeight / rowHeight) + overscanRows * 2 + virtualWindowStrideRows - 1;
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
    cardWidth,
    rowHeight,
    needsPrevious: globalStartIndex < retainedStartIndex,
    needsNext: globalEndIndex > retainedEndIndex
  };
}
