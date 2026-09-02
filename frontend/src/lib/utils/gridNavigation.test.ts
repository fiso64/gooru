import { describe, expect, it } from 'vitest';
import { nextGridIndex, type GridRect } from './gridNavigation';

const regularGrid: GridRect[] = [
  { left: 0, top: 0, width: 100, height: 80 },
  { left: 110, top: 0, width: 100, height: 80 },
  { left: 220, top: 0, width: 100, height: 80 },
  { left: 0, top: 90, width: 100, height: 80 },
  { left: 110, top: 90, width: 100, height: 80 },
  { left: 220, top: 90, width: 100, height: 80 }
];

describe('nextGridIndex', () => {
  it('moves spatially across rows and columns', () => {
    expect(nextGridIndex(regularGrid, 1, 'ArrowLeft')).toBe(0);
    expect(nextGridIndex(regularGrid, 1, 'ArrowRight')).toBe(2);
    expect(nextGridIndex(regularGrid, 1, 'ArrowDown')).toBe(4);
    expect(nextGridIndex(regularGrid, 4, 'ArrowUp')).toBe(1);
  });

  it('stays put at an outer edge', () => {
    expect(nextGridIndex(regularGrid, 0, 'ArrowLeft')).toBe(0);
    expect(nextGridIndex(regularGrid, 2, 'ArrowUp')).toBe(2);
  });

  it('prefers the visually aligned candidate for irregular chip widths', () => {
    const chips: GridRect[] = [
      { left: 0, top: 0, width: 70, height: 32 },
      { left: 80, top: 0, width: 150, height: 32 },
      { left: 0, top: 44, width: 110, height: 32 },
      { left: 120, top: 44, width: 90, height: 32 }
    ];
    expect(nextGridIndex(chips, 1, 'ArrowDown')).toBe(3);
  });
});
