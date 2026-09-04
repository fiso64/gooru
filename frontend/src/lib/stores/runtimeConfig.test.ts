import { describe, expect, it } from 'vitest';
import { normalizeThumbnailSizes } from './runtimeConfig';

describe('normalizeThumbnailSizes', () => {
  it('filters, deduplicates, and sorts the configured catalogue once', () => {
    expect(normalizeThumbnailSizes([512, 256, 512, 0, -1, Number.NaN, Number.POSITIVE_INFINITY, 1024])).toEqual([256, 512, 1024]);
  });
});
