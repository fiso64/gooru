import { describe, expect, it } from 'vitest';
import type { FileItem } from '$lib/api/types';
import { gridCardWidth, gridColumns, gridRowHeight, virtualGrid, virtualGridStartRow, virtualMediaLayout } from './ui';

function mediaFile(id: string, width: number, height: number): FileItem {
  return { id, content_id: `hash-${id}`, name: `${id}.jpg`, safe_display_path: `library/${id}.jpg`, size: 1,
    added_at: '2026-01-01T00:00:00Z', modified_time: '2026-01-01T00:00:00Z', media_type: 'image/jpeg', media_kind: 'photo',
    metadata: { image_width: width, image_height: height }, tags: [], media_urls: { thumbnail: '/thumbnail', preview: '/preview', content: '/content', download: '/download' }, can_delete: false };
}

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
  it('uses one shared configured card width for layout and thumbnail sizing', () => {
    expect(gridColumns(960, 180)).toBe(5); expect(gridColumns(960, 240)).toBe(3);
    const expectedCardWidth = (960 - 32 - 5 * 2) / 3;
    expect(gridCardWidth(960, 240, 3)).toBeCloseTo(expectedCardWidth);
    const configured = virtualGrid([], 960, 800, 0, 0, 30, 0, 240);
    expect(configured.columns).toBe(3); expect(configured.cardWidth).toBeCloseTo(expectedCardWidth);
    expect(configured.rowHeight).toBeCloseTo(expectedCardWidth + 5); expect(configured.totalHeight).toBeCloseTo(configured.rowHeight * 10);
  });
});

describe('tile gallery layout', () => {
  const files = [mediaFile('a', 1600, 900), mediaFile('b', 600, 900), mediaFile('c', 900, 900), mediaFile('d', 900, 1400), mediaFile('e', 1200, 800), mediaFile('f', 700, 1200)];
  it('preserves source aspect ratios and source order', () => {
    const layout = virtualMediaLayout(files, 1000, 800, 0, 0, files.length, 0, 180);
    expect(layout.items.map((item) => item.file.id)).toEqual(files.map((item) => item.id));
    for (const item of layout.items) expect(item.width / item.height).toBeCloseTo(item.file.metadata.image_width! / item.file.metadata.image_height!, 5);
  });
  it('estimates unloaded library height without expanding retained DOM work', () => {
    const layout = virtualMediaLayout(files, 1000, 800, 0, 0, 10_000, 0, 180);
    expect(layout.totalHeight).toBeGreaterThan(100_000); expect(layout.items.length).toBeLessThanOrEqual(files.length); expect(layout.needsNext).toBe(true);
  });
});
