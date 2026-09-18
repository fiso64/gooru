import { describe, expect, it } from 'vitest';
import { nextGridIndex, type GridRect } from './gridNavigation';

const grid: GridRect[] = [
  { left: 0, top: 0, width: 100, height: 80 },
  { left: 110, top: 0, width: 100, height: 80 },
  { left: 220, top: 0, width: 100, height: 80 },
  { left: 0, top: 90, width: 100, height: 80 },
  { left: 110, top: 90, width: 100, height: 80 },
  { left: 220, top: 90, width: 100, height: 80 }
];

describe('grid horizontal row wrapping', () => {
  it('moves from a row end to the next row start and back', () => {
    expect(nextGridIndex(grid, 2, 'ArrowRight', { wrapHorizontal: true })).toBe(3);
    expect(nextGridIndex(grid, 3, 'ArrowLeft', { wrapHorizontal: true })).toBe(2);
  });

  it('does not wrap beyond the first or final item', () => {
    expect(nextGridIndex(grid, 0, 'ArrowLeft', { wrapHorizontal: true })).toBe(0);
    expect(nextGridIndex(grid, 5, 'ArrowRight', { wrapHorizontal: true })).toBe(5);
  });

  it('keeps the default shared-grid edge behavior unchanged', () => {
    expect(nextGridIndex(grid, 2, 'ArrowRight')).toBe(2);
    expect(nextGridIndex(grid, 3, 'ArrowLeft')).toBe(3);
  });
});
