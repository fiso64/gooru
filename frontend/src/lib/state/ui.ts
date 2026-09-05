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

export interface VirtualMediaItem {
  file: FileItem;
  index: number;
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface VirtualMediaLayout {
  items: VirtualMediaItem[];
  totalHeight: number;
  needsPrevious: boolean;
  needsNext: boolean;
}

const gridPadding = 16 * 2;
const gridInset = 16;
const gridGap = 5;
const overscanRows = 4;
const virtualWindowStrideRows = 3;
const variableOverscanScreens = 1;

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

function mediaAspect(file: FileItem) {
  const width = file.metadata.image_width ?? file.metadata.video_width ?? 0;
  const height = file.metadata.image_height ?? file.metadata.video_height ?? 0;
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) return 1;
  return Math.min(8, Math.max(0.125, width / height));
}

function tilePlacements(files: FileItem[], containerWidth: number, minCardWidth: number) {
  const innerWidth = Math.max(minCardWidth, containerWidth - gridPadding);
  const placements: VirtualMediaItem[] = [];
  let y = gridInset;
  let start = 0;
  while (start < files.length) {
    let end = start;
    let aspectSum = 0;
    while (end < files.length) {
      aspectSum += mediaAspect(files[end]);
      end += 1;
      const widthAtTarget = aspectSum * minCardWidth + Math.max(0, end - start - 1) * gridGap;
      if (widthAtTarget >= innerWidth) break;
    }
    const count = end - start;
    const isLast = end === files.length;
    const availableWidth = innerWidth - Math.max(0, count - 1) * gridGap;
    const exactHeight = availableWidth / Math.max(aspectSum, 0.001);
    const rowHeight = isLast && exactHeight > minCardWidth * 1.15 ? minCardWidth : exactHeight;
    let x = gridInset;
    for (let index = start; index < end; index += 1) {
      const width = rowHeight * mediaAspect(files[index]);
      placements.push({ file: files[index], index, x, y, width, height: rowHeight });
      x += width + gridGap;
    }
    y += rowHeight + gridGap;
    start = end;
  }
  return { placements, height: Math.max(gridInset * 2, y - gridGap + gridInset) };
}

export function virtualMediaLayout(
  files: FileItem[],
  containerWidth: number,
  viewportHeight: number,
  scrollY: number,
  gridTop: number,
  totalItems = files.length,
  retainedStartIndex = 0,
  minCardWidth = defaultGridSize
): VirtualMediaLayout {
  const local = tilePlacements(files, containerWidth, minCardWidth);
  const retainedCount = Math.max(1, files.length);
  const localContentHeight = Math.max(1, local.height - gridPadding);
  const heightPerItem = localContentHeight / retainedCount;
  const prefixHeight = retainedStartIndex * heightPerItem;
  const retainedEndIndex = retainedStartIndex + files.length;
  const suffixCount = Math.max(0, totalItems - retainedEndIndex);
  const totalHeight = Math.max(viewportHeight, prefixHeight + local.height + suffixCount * heightPerItem);
  const viewportStart = Math.max(0, scrollY - gridTop);
  const overscan = Math.max(minCardWidth * 2, viewportHeight * variableOverscanScreens);
  const startY = Math.max(0, viewportStart - overscan);
  const endY = viewportStart + viewportHeight + overscan;
  const retainedTop = prefixHeight;
  const retainedBottom = prefixHeight + local.height;
  const items = local.placements
    .map((item) => ({ ...item, index: retainedStartIndex + item.index, y: retainedTop + item.y }))
    .filter((item) => item.y + item.height >= startY && item.y <= endY);
  return {
    items,
    totalHeight,
    needsPrevious: retainedStartIndex > 0 && startY <= retainedTop + overscan,
    needsNext: retainedEndIndex < totalItems && endY >= retainedBottom - overscan
  };
}
