import { describe, expect, it } from 'vitest';
import type { FileItem } from '$lib/api/types';
import { gridCardWidth, gridColumns, gridRowHeight, virtualGrid, virtualGridStartRow, virtualMediaGeometry, virtualMediaLayout } from './ui';

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
  it('keeps published rows prefix-stable when a non-aligned query page is appended', () => {
    const dimensions = [[1600, 900], [600, 900], [900, 900], [900, 1400], [1200, 800], [700, 1200], [2000, 800]] as const;
    const pagedFiles = Array.from({ length: 120 }, (_, index) => {
      const [width, height] = dimensions[index % dimensions.length];
      return mediaFile(`page-${index}`, width, height);
    });
    const firstPage = virtualMediaGeometry(pagedFiles.slice(0, 60), 1200, 120, 0, 200);

    // This aspect sequence makes items 57-59 an incomplete transport-page tail. Publishing
    // that row before page 2 arrives would make those visible cards move on the append.
    expect(firstPage.placements.at(-1)?.index).toBe(56);

    const twoPages = virtualMediaGeometry(pagedFiles, 1200, 120, 0, 200);
    for (const before of firstPage.placements) {
      const after = twoPages.placements.find((item) => item.index === before.index);
      expect(after).toBeDefined();
      expect(after).toMatchObject({ x: before.x, y: before.y, width: before.width, height: before.height });
    }
  });
  it('flushes an incomplete row at the actual end of the result set', () => {
    const tail = [mediaFile('tail-a', 900, 900), mediaFile('tail-b', 900, 900), mediaFile('tail-c', 900, 900)];
    const geometry = virtualMediaGeometry(tail, 1200, tail.length, 0, 200);
    expect(geometry.placements).toHaveLength(tail.length);
  });
});
