import { describe, expect, it } from 'vitest';
import {
  defaultGridSize,
  denseGridSizeBoost,
  effectiveGridSize,
  normalizeConfiguredUITheme,
  normalizeThumbnailSizes,
  normalizeUITheme
} from './runtimeConfig';

describe('normalizeThumbnailSizes', () => {
  it('filters, deduplicates, and sorts the configured catalogue once', () => {
    expect(normalizeThumbnailSizes([512, 256, 512, 0, -1, Number.NaN, Number.POSITIVE_INFINITY, 1024])).toEqual([256, 512, 1024]);
  });
});

describe('UI theme normalization', () => {
  it('maps both booru variants onto the shared booru presentation', () => {
    expect(normalizeConfiguredUITheme('booru-light')).toBe('booru-light');
    expect(normalizeConfiguredUITheme('booru-dark')).toBe('booru-dark');
    expect(normalizeUITheme('booru-light')).toBe('booru-style');
    expect(normalizeUITheme('booru-dark')).toBe('booru-style');
  });

  it('does not retain the renamed booru-style config value', () => {
    expect(normalizeConfiguredUITheme('booru-style')).toBe('default');
    expect(normalizeUITheme('booru-style')).toBe('default');
  });

  it('keeps the existing presentation as the safe default', () => {
    expect(normalizeUITheme(undefined)).toBe('default');
    expect(normalizeUITheme('unknown')).toBe('default');
  });
});

describe('grid layout sizing', () => {
  it('keeps the standard theme 200px default', () => {
    expect(defaultGridSize).toBe(200);
  });

  it('keeps square-fill at the configured size and boosts square-fit and tile layouts', () => {
    expect(denseGridSizeBoost).toBe(40);
    expect(effectiveGridSize(200, 'square')).toBe(200);
    expect(effectiveGridSize(200, 'fit')).toBe(240);
    expect(effectiveGridSize(200, 'tile')).toBe(240);
  });
});
