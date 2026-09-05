import { describe, expect, it } from 'vitest';
import { defaultGridSize, denseGridSizeBoost, effectiveGridSize, normalizeThumbnailSizes } from './runtimeConfig';

describe('normalizeThumbnailSizes', () => {
  it('filters, deduplicates, and sorts the configured catalogue once', () => {
    expect(normalizeThumbnailSizes([512, 256, 512, 0, -1, Number.NaN, Number.POSITIVE_INFINITY, 1024])).toEqual([256, 512, 1024]);
  });
});

describe('grid layout sizing', () => {
  it('uses the requested 200px default', () => {
    expect(defaultGridSize).toBe(200);
  });

  it('keeps square-fill at the configured size and boosts square-fit and tile layouts', () => {
    expect(denseGridSizeBoost).toBe(40);
    expect(effectiveGridSize(200, 'square')).toBe(200);
    expect(effectiveGridSize(200, 'fit')).toBe(240);
    expect(effectiveGridSize(200, 'tile')).toBe(240);
  });
});
