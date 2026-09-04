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

  it('boosts square and tile layouts while leaving fit at the configured size', () => {
    expect(denseGridSizeBoost).toBe(40);
    expect(effectiveGridSize(200, 'square')).toBe(240);
    expect(effectiveGridSize(200, 'tile')).toBe(240);
    expect(effectiveGridSize(200, 'fit')).toBe(200);
  });
});
