import { writable } from 'svelte/store';

export const defaultGridSize = 180;

export type RuntimeConfig = {
  loadFullMediaByDefault: boolean;
  gridSize: number;
  thumbnailSizes: number[];
};

export function normalizeThumbnailSizes(sizes: number[]) {
  return [...new Set(sizes.filter((size) => Number.isFinite(size) && size > 0))].sort((a, b) => a - b);
}

export const runtimeConfig = writable<RuntimeConfig>({
  loadFullMediaByDefault: false,
  gridSize: defaultGridSize,
  thumbnailSizes: []
});
