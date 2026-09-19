import { writable } from 'svelte/store';
import type { ViewerConfiguredFitMode } from '$lib/utils/viewer';
import type { ViewerScaling } from '$lib/state/viewerSessionPreferences';

export const defaultGridSize = 200;
export const denseGridSizeBoost = 40;
export const defaultItemsPerPage = 60;
export type GridType = 'square' | 'fit' | 'tile';
export type PaginationMode = 'infinite' | 'paged';
export type ConfiguredUITheme = 'default' | 'booru-light' | 'booru-dark';
// The two booru variants share one shell/behavior boundary. Their visual variant is
// carried separately so the existing booru presentation remains a single frontend.
export type UITheme = 'default' | 'booru-style';

export const runtimeCapability = {
  previewImages: 'preview_images'
} as const;

export type RuntimeCapability = (typeof runtimeCapability)[keyof typeof runtimeCapability];

export type RuntimeConfig = {
  uiTheme: UITheme;
  capabilities: string[];
  loadFullMediaByDefault: boolean;
  preferLosslessFullImage: boolean;
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

export function normalizeConfiguredUITheme(value: string | undefined): ConfiguredUITheme {
  return value === 'booru-light' || value === 'booru-dark' ? value : 'default';
}

export function normalizeUITheme(value: string | undefined): UITheme {
  return value === 'booru-light' || value === 'booru-dark' ? 'booru-style' : 'default';
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
  uiTheme: 'default',
  capabilities: [runtimeCapability.previewImages],
  loadFullMediaByDefault: false,
  preferLosslessFullImage: true,
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
