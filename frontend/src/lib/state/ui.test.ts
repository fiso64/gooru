import { describe, expect, it } from 'vitest';
import type { FileItem } from '$lib/api/types';
import { gridCardWidth, gridColumns, gridRowHeight, virtualGrid, virtualGridStartRow, virtualMediaGeometry, virtualMediaLayout } from './ui';

function mediaFile(id: string, width: number, height: number): FileItem {
  return { id, content_id: `hash-${id}`, name: `${id}.jpg`, safe_display_path: `library/${id}.jpg`, size: 1,
    added_at: '2026-01-01T00:00:00Z', modified_time: '2026-01-01T00:00:00Z', media_type: 'image/jpeg', media_kind: 'photo', viewer_support: 'supported',
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
  it('changes the rendered slice only when the virtual window crosses a row boundary', () => {
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
  it('keeps startup on one page but requests page 2 four rows ahead of the retained tail', () => {
    const files = Array.from({ length: 60 }, (_, index) => ({ id: `file-${index}` })) as never[];
    const initial = virtualGrid(files, 1440, 900, 0, 100, 10_000, 0, 200);
    expect(initial.columns).toBe(6);
    expect(initial.files).toHaveLength(48);
    expect(initial.needsNext).toBe(false);

    const twoRowsDown = virtualGrid(files, 1440, 900, initial.rowHeight * 2, 100, 10_000, 0, 200);
    expect(twoRowsDown.needsNext).toBe(false);
    const threeRowsDown = virtualGrid(files, 1440, 900, initial.rowHeight * 3, 100, 10_000, 0, 200);
    expect(threeRowsDown.needsNext).toBe(true);
  });
});

describe('tile gallery layout', () => {
  const files = [mediaFile('a', 1600, 900), mediaFile('b', 600, 900), mediaFile('c', 900, 900), mediaFile('d', 900, 1400), mediaFile('e', 1200, 800), mediaFile('f', 700, 1200)];
  it('preserves source aspect ratios and source order', () => {
    const layout = virtualMediaLayout(files, 1000, 800, 0, 0, files.length, 0, 180);
    expect(layout.items.map((item) => item.file.id)).toEqual(files.map((item) => item.id));
    for (const item of layout.items) expect(item.width / item.height).toBeCloseTo(item.file.metadata.image_width! / item.file.metadata.image_height!, 5);
  });
  it('uses an observed thumbnail aspect when source dimensions are unavailable', () => {
    const file = mediaFile('missing-dimensions', 1200, 1600);
    file.metadata = {};
    const geometry = virtualMediaGeometry([file], 1000, 1, 0, 256, { [file.id]: 256 / 341 });
    expect(geometry.placements).toHaveLength(1);
    expect(geometry.placements[0].width / geometry.placements[0].height).toBeCloseTo(256 / 341, 5);
  });
  it('keeps valid source dimensions authoritative over an observed thumbnail aspect', () => {
    const file = mediaFile('known-dimensions', 1200, 1600);
    const geometry = virtualMediaGeometry([file], 1000, 1, 0, 256, { [file.id]: 1 });
    expect(geometry.placements[0].width / geometry.placements[0].height).toBeCloseTo(1200 / 1600, 5);
  });
  it('avoids collapsing a row when the crossing item overshoots the target height', () => {
    const wide = [mediaFile('wide-a', 3000, 1000), mediaFile('wide-b', 3000, 1000)];
    const geometry = virtualMediaGeometry(wide, 1000, wide.length, 0, 256);
    expect(geometry.placements).toHaveLength(2);
    expect(geometry.placements[1].y).toBeGreaterThan(geometry.placements[0].y);
    expect(geometry.placements[0].height).toBeGreaterThan(250);
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

    expect(firstPage.placements.at(-1)?.index).toBeLessThan(59);

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
