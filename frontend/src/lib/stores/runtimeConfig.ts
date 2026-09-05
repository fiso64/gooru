import { writable } from 'svelte/store';
import type { ViewerConfiguredFitMode } from '$lib/utils/viewer';

export const defaultGridSize = 180;

export type RuntimeConfig = {
  loadFullMediaByDefault: boolean;
  fullscreenMediaByDefault: boolean;
  viewerFitMode: ViewerConfiguredFitMode;
  gridSize: number;
  thumbnailSizes: number[];
};

export function normalizeThumbnailSizes(sizes: number[]) {
  return [...new Set(sizes.filter((size) => Number.isFinite(size) && size > 0))].sort((a, b) => a - b);
}

export const runtimeConfig = writable<RuntimeConfig>({
  loadFullMediaByDefault: false,
  fullscreenMediaByDefault: false,
  viewerFitMode: 'fit_window',
  gridSize: defaultGridSize,
  thumbnailSizes: []
});
