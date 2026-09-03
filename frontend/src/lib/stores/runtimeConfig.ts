import { writable } from 'svelte/store';

export const defaultGridSize = 180;
export const defaultThumbnailSizes = [256, 512];

export type RuntimeConfig = {
  loadFullMediaByDefault: boolean;
  gridSize: number;
  thumbnailSizes: number[];
};

export const runtimeConfig = writable<RuntimeConfig>({
  loadFullMediaByDefault: false,
  gridSize: defaultGridSize,
  thumbnailSizes: defaultThumbnailSizes
});
