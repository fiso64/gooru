import { writable } from 'svelte/store';
import type { ViewerConfiguredFitMode } from '$lib/utils/viewer';
import type { ViewerScaling } from '$lib/state/viewerSessionPreferences';

export const defaultGridSize = 180;

export type RuntimeConfig = {
  loadFullMediaByDefault: boolean;
  fullscreenMediaByDefault: boolean;
  viewerFitMode: ViewerConfiguredFitMode;
  viewerScaling: ViewerScaling;
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
  viewerScaling: 'smooth',
  gridSize: defaultGridSize,
  thumbnailSizes: []
});
