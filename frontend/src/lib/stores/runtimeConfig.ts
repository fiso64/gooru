import { writable } from 'svelte/store';
import type { ViewerConfiguredFitMode } from '$lib/utils/viewer';
import type { ViewerScaling } from '$lib/state/viewerSessionPreferences';

export const defaultGridSize = 200;
export const denseGridSizeBoost = 40;
export const defaultItemsPerPage = 60;
export type GridType = 'square' | 'fit' | 'tile';
export type PaginationMode = 'infinite' | 'paged';

export const runtimeCapability = {
  previewImages: 'preview_images'
} as const;

export type RuntimeCapability = (typeof runtimeCapability)[keyof typeof runtimeCapability];

export type RuntimeConfig = {
  capabilities: string[];
  loadFullMediaByDefault: boolean;
  fullscreenMediaByDefault: boolean;
  hoverPlayVideos: boolean;
  hoverPlayGifs: boolean;
  viewerFitMode: ViewerConfiguredFitMode;
  viewerActualSizeFitCap: boolean;
  viewerScaling: ViewerScaling;
  gridSize: number;
  gridType: GridType;
  thumbnailSizes: number[];
  paginationMode: PaginationMode;
  itemsPerPage: number;
};

export function normalizeThumbnailSizes(values: number[]): number[] {
  return [...new Set(values.filter((value) => Number.isFinite(value) && value > 0).map((value) => Math.round(value)))].sort((a, b) => a - b);
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

export function hasRuntimeCapability(config: RuntimeConfig, capability: RuntimeCapability): boolean {
  return config.capabilities.includes(capability);
}

export const runtimeConfig = writable<RuntimeConfig>({
  capabilities: [runtimeCapability.previewImages],
  loadFullMediaByDefault: false,
  fullscreenMediaByDefault: false,
  hoverPlayVideos: true,
  hoverPlayGifs: true,
  viewerFitMode: 'fit_window',
  viewerActualSizeFitCap: true,
  viewerScaling: 'smooth',
  gridSize: defaultGridSize,
  gridType: 'square',
  thumbnailSizes: [],
  paginationMode: 'infinite',
  itemsPerPage: defaultItemsPerPage
});
