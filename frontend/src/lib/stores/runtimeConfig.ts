import { writable } from 'svelte/store';
import type { ViewerConfiguredFitMode } from '$lib/utils/viewer';
import type { ViewerScaling } from '$lib/state/viewerSessionPreferences';

export const defaultGridSize = 200;
export const denseGridSizeBoost = 40;
export const defaultItemsPerPage = 60;
export type GridType = 'square' | 'fit' | 'tile';
export type PaginationMode = 'infinite' | 'paged';

export type RuntimeConfig = {
  loadFullMediaByDefault: boolean;
  fullscreenMediaByDefault: boolean;
  viewerFitMode: ViewerConfiguredFitMode;
  viewerScaling: ViewerScaling;
  gridSize: number;
  gridType: GridType;
  thumbnailSizes: number[];
  paginationMode: PaginationMode;
  itemsPerPage: number;
};

export function normalizeThumbnailSizes(values: number[]): number[] {
  return values
    .filter((value) => Number.isFinite(value) && value > 0)
    .map((value) => Math.round(value))
    .sort((a, b) => a - b);
}

export function normalizeGridType(value: string | undefined): GridType {
  return value === 'fit' || value === 'tile' ? value : 'square';
}

export function normalizePaginationMode(value: string | undefined): PaginationMode {
  return value === 'paged' ? 'paged' : 'infinite';
}

export function normalizeItemsPerPage(value: number | undefined): number {
  if (!Number.isFinite(value) || value === undefined) return defaultItemsPerPage;
  return Math.max(1, Math.min(200, Math.round(value)));
}

export function effectiveGridSize(size: number, gridType: GridType): number {
  return gridType === 'square' ? size : size + denseGridSizeBoost;
}

export const runtimeConfig = writable<RuntimeConfig>({
  loadFullMediaByDefault: false,
  fullscreenMediaByDefault: false,
  viewerFitMode: 'fit_window',
  viewerScaling: 'smooth',
  gridSize: defaultGridSize,
  gridType: 'square',
  thumbnailSizes: [],
  paginationMode: 'infinite',
  itemsPerPage: defaultItemsPerPage
});
