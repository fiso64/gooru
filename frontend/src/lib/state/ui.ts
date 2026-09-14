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

export interface VirtualMediaGeometry {
  placements: VirtualMediaItem[];
  localHeight: number;
  retainedStartIndex: number;
  retainedEndIndex: number;
  totalItems: number;
  heightPerItem: number;
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
const transportLoadAheadRows = 4;
const variableOverscanScreens = 1;

type MediaAspectOverrides = Readonly<Record<string, number>>;

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
  const viewportStart = Math.max(0, scrollY - gridTop);
  const startRow = virtualGridStartRow(scrollY, gridTop, rowHeight);
  const viewportEndRow = Math.ceil((viewportStart + viewportHeight) / rowHeight);
  const endRow = Math.min(totalRows, viewportEndRow + overscanRows);
  const retainedEndIndex = retainedStartIndex + files.length;
  const globalStartIndex = startRow * columns;
  const globalEndIndex = endRow * columns;
  const transportEndRow = Math.min(totalRows, Math.ceil((viewportStart + viewportHeight) / rowHeight) + transportLoadAheadRows);
  const transportEndIndex = transportEndRow * columns;
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
    // DOM overscan intentionally spans several rows to keep scrolling smooth, but using that
    // same window as a transport trigger eagerly fetched page 2 on a normal 60-item startup.
    // Keep network look-ahead independent so the next page is requested several rows before
    // the viewport reaches the retained tail without doubling initial list or thumbnail work.
    needsNext: transportEndIndex > retainedEndIndex
  };
}

function mediaAspect(file: FileItem, aspectOverrides?: MediaAspectOverrides) {
  const width = file.metadata.image_width ?? file.metadata.video_width ?? 0;
  const height = file.metadata.image_height ?? file.metadata.video_height ?? 0;
  if (Number.isFinite(width) && Number.isFinite(height) && width > 0 && height > 0) {
    return Math.min(8, Math.max(0.125, width / height));
  }
  const observedAspect = aspectOverrides?.[file.id] ?? 0;
  if (Number.isFinite(observedAspect) && observedAspect > 0) {
    return Math.min(8, Math.max(0.125, observedAspect));
  }
  return 1;
}

function rowHeightFor(innerWidth: number, count: number, aspectSum: number) {
  const availableWidth = innerWidth - Math.max(0, count - 1) * gridGap;
  return availableWidth / Math.max(aspectSum, 0.001);
}

function tilePlacements(
  files: FileItem[],
  containerWidth: number,
  minCardWidth: number,
  flushIncompleteRow: boolean,
  aspectOverrides?: MediaAspectOverrides
) {
  const innerWidth = Math.max(minCardWidth, containerWidth - gridPadding);
  const placements: VirtualMediaItem[] = [];
  let y = gridInset;
  let start = 0;
  while (start < files.length) {
    let end = start;
    let aspectSum = 0;
    let rowFilled = false;
    while (end < files.length) {
      const nextAspect = mediaAspect(files[end], aspectOverrides);
      const nextCount = end - start + 1;
      const nextAspectSum = aspectSum + nextAspect;
      const widthAtTarget = nextAspectSum * minCardWidth + Math.max(0, nextCount - 1) * gridGap;

      if (widthAtTarget >= innerWidth && end > start) {
        const previousCount = end - start;
        const previousHeight = rowHeightFor(innerWidth, previousCount, aspectSum);
        const nextHeight = rowHeightFor(innerWidth, nextCount, nextAspectSum);
        if (Math.abs(previousHeight - minCardWidth) < Math.abs(nextHeight - minCardWidth)) {
          rowFilled = true;
          break;
        }
      }

      aspectSum = nextAspectSum;
      end += 1;
      if (widthAtTarget >= innerWidth) {
        rowFilled = true;
        break;
      }
    }

    // Infinite-query pages are transport chunks, not layout boundaries. If the loaded
    // sequence ends before this row has enough media to close naturally, keep the tail
    // pending until another page arrives. Publishing it now would make its membership and
    // geometry change on append, moving already-visible cards on every non-aligned page turn.
    if (!rowFilled && !flushIncompleteRow) break;

    const count = end - start;
    const isLast = end === files.length;
    const exactHeight = rowHeightFor(innerWidth, count, aspectSum);
    const rowHeight = isLast && exactHeight > minCardWidth * 1.15 ? minCardWidth : exactHeight;
    let x = gridInset;
    for (let index = start; index < end; index += 1) {
      const width = rowHeight * mediaAspect(files[index], aspectOverrides);
      placements.push({ file: files[index], index, x, y, width, height: rowHeight });
      x += width + gridGap;
    }
    y += rowHeight + gridGap;
    start = end;
  }
  return { placements, height: Math.max(gridInset * 2, y - gridGap + gridInset) };
}

// Geometry depends only on the logical media sequence and grid dimensions. Keeping it
// separate from viewport selection means normal scroll events do not repack every loaded
// tile; geometry is recomputed only when files or layout dimensions actually change.
export function virtualMediaGeometry(
  files: FileItem[],
  containerWidth: number,
  totalItems = files.length,
  retainedStartIndex = 0,
  minCardWidth = defaultGridSize,
  aspectOverrides?: MediaAspectOverrides
): VirtualMediaGeometry {
  const retainedEndIndex = retainedStartIndex + files.length;
  const local = tilePlacements(files, containerWidth, minCardWidth, retainedEndIndex >= totalItems, aspectOverrides);
  const retainedCount = Math.max(1, files.length);
  const localContentHeight = Math.max(1, local.height - gridPadding);
  return {
    placements: local.placements.map((item) => ({ ...item, index: retainedStartIndex + item.index })),
    localHeight: local.height,
    retainedStartIndex,
    retainedEndIndex,
    totalItems,
    heightPerItem: localContentHeight / retainedCount
  };
}

function firstPlacementEndingAtOrAfter(placements: VirtualMediaItem[], y: number) {
  let low = 0;
  let high = placements.length;
  while (low < high) {
    const mid = Math.floor((low + high) / 2);
    const item = placements[mid];
    if (item.y + item.height < y) low = mid + 1;
    else high = mid;
  }
  return low;
}

function firstPlacementStartingAfter(placements: VirtualMediaItem[], y: number) {
  let low = 0;
  let high = placements.length;
  while (low < high) {
    const mid = Math.floor((low + high) / 2);
    if (placements[mid].y <= y) low = mid + 1;
    else high = mid;
  }
  return low;
}

export function virtualMediaWindow(
  geometry: VirtualMediaGeometry,
  viewportHeight: number,
  scrollY: number,
  gridTop: number,
  minCardWidth = defaultGridSize
): VirtualMediaLayout {
  const prefixHeight = geometry.retainedStartIndex * geometry.heightPerItem;
  const suffixCount = Math.max(0, geometry.totalItems - geometry.retainedEndIndex);
  const totalHeight = Math.max(viewportHeight, prefixHeight + geometry.localHeight + suffixCount * geometry.heightPerItem);
  const viewportStart = Math.max(0, scrollY - gridTop);
  const overscan = Math.max(minCardWidth * 2, viewportHeight * variableOverscanScreens);
  const startY = Math.max(0, viewportStart - overscan);
  const endY = viewportStart + viewportHeight + overscan;
  const retainedTop = prefixHeight;
  const retainedBottom = prefixHeight + geometry.localHeight;
  const localStartY = startY - retainedTop;
  const localEndY = endY - retainedTop;
  const first = firstPlacementEndingAtOrAfter(geometry.placements, localStartY);
  const last = firstPlacementStartingAfter(geometry.placements, localEndY);
  const items = geometry.placements.slice(first, last).map((item) => ({ ...item, y: retainedTop + item.y }));
  return {
    items,
    totalHeight,
    needsPrevious: geometry.retainedStartIndex > 0 && startY <= retainedTop + overscan,
    needsNext: geometry.retainedEndIndex < geometry.totalItems && endY >= retainedBottom - overscan
  };
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
  return virtualMediaWindow(
    virtualMediaGeometry(files, containerWidth, totalItems, retainedStartIndex, minCardWidth),
    viewportHeight,
    scrollY,
    gridTop,
    minCardWidth
  );
}
