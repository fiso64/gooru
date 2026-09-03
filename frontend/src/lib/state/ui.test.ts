import { describe, expect, it } from 'vitest';
import { gridColumns, gridRowHeight, virtualGrid, virtualGridStartRow } from './ui';

describe('virtual grid scrolling', () => {
  it('keeps the reactive window stable across scroll events within overscan rows', () => {
    const rowHeight = gridRowHeight(960);
    expect(virtualGridStartRow(0, 120, rowHeight)).toBe(0);
    expect(virtualGridStartRow(120 + rowHeight * 4.9, 120, rowHeight)).toBe(0);
    expect(virtualGridStartRow(120 + rowHeight * 5.1, 120, rowHeight)).toBe(0);
    expect(virtualGridStartRow(120 + rowHeight * 7.1, 120, rowHeight)).toBe(3);
  });

  it('changes the rendered slice only when the virtual start row changes', () => {
    const files = Array.from({ length: 240 }, (_, index) => ({ id: `file-${index}` })) as never[];
    const first = virtualGrid(files, 960, 800, 700, 100, 10_000, 0);
    const sameWindow = virtualGrid(files, 960, 800, 760, 100, 10_000, 0);
    const nextWindow = virtualGrid(files, 960, 800, 900, 100, 10_000, 0);
    expect(sameWindow.offsetTop).toBe(first.offsetTop);
    expect(sameWindow.files.map((file: { id: string }) => file.id)).toEqual(first.files.map((file: { id: string }) => file.id));
    expect(nextWindow.offsetTop).toBeGreaterThanOrEqual(first.offsetTop);
  });

  it('uses the configured minimum cell size for both columns and row geometry', () => {
    expect(gridColumns(960, 180)).toBe(5);
    expect(gridColumns(960, 240)).toBe(3);

    const configured = virtualGrid([], 960, 800, 0, 0, 30, 0, 240);
    expect(configured.columns).toBe(3);
    expect(configured.rowHeight).toBeCloseTo((960 - 32 - 5 * 2) / 3 + 5);
    expect(configured.totalHeight).toBeCloseTo(configured.rowHeight * 10);
  });
});
