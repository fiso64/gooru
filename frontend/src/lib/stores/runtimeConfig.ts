import { writable } from 'svelte/store';

export const defaultGridSize = 200;
export const denseGridSizeBoost = 40;
export type GridType = 'square' | 'fit' | 'tile';

export type RuntimeConfig = {
  loadFullMediaByDefault: boolean;
  fullscreenMediaByDefault: boolean;
  gridSize: number;
  gridType: GridType;
  thumbnailSizes: number[];
};

export function normalizeThumbnailSizes(sizes: number[]) {
  return [...new Set(sizes.filter((size) => Number.isFinite(size) && size > 0))].sort((a, b) => a - b);
}

export function normalizeGridType(value: string | undefined): GridType {
  return value === 'fit' || value === 'tile' ? value : 'square';
}

export function effectiveGridSize(size: number, type: GridType) {
  return size + (type === 'fit' ? 0 : denseGridSizeBoost);
}

export const runtimeConfig = writable<RuntimeConfig>({
  loadFullMediaByDefault: false,
  fullscreenMediaByDefault: false,
  gridSize: defaultGridSize,
  gridType: 'square',
  thumbnailSizes: []
});
