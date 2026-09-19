import { describe, expect, it } from 'vitest';
import { adjacentComicPages, comicPageAt, comicPageSource, isComicFile, moveComicPage } from './comic';
import type { ComicManifest } from '$lib/api/types';

const manifest: ComicManifest = {
  pages: [
    { index: 0, name: '1.png', url: '/api/v1/comics/book/0' },
    { index: 1, name: '2.png', url: '/api/v1/comics/book/1' },
    { index: 2, name: '10.png', url: '/api/v1/comics/book/2' }
  ]
};

describe('comic viewer policy', () => {
  it('recognizes the canonical comic MIME type and CBZ filename fallback', () => {
    expect(isComicFile({ name: 'book.bin', media_type: 'application/vnd.comicbook+zip' })).toBe(true);
    expect(isComicFile({ name: 'BOOK.CBZ', media_type: 'application/octet-stream' })).toBe(true);
    expect(isComicFile({ name: 'photo.jpg', media_type: 'image/jpeg' })).toBe(false);
  });

  it('clamps page navigation at both ends', () => {
    expect(moveComicPage(0, -1, 3)).toBe(0);
    expect(moveComicPage(0, 1, 3)).toBe(1);
    expect(moveComicPage(2, 1, 3)).toBe(2);
    expect(moveComicPage(1, 20, 3)).toBe(2);
  });

  it('returns only real adjacent pages for preloading', () => {
    expect(adjacentComicPages(manifest, 0).map((page) => page.index)).toEqual([1]);
    expect(adjacentComicPages(manifest, 1).map((page) => page.index)).toEqual([0, 2]);
  });

  it('selects preview, optional lossless full image and always preserves the actual original', () => {
    const page = {
      index: 0,
      name: 'page.jpg',
      url: '/api/v1/comics/book/0',
      preview: '/api/v1/comics/book/0?variant=preview',
      lossless: '/api/v1/comics/book/0?variant=lossless'
    };
    expect(comicPageSource(page, false, true)).toBe(page.preview);
    expect(comicPageSource(page, true, true)).toBe(page.lossless);
    expect(comicPageSource(page, true, false)).toBe(page.url);
    expect(comicPageSource({ ...page, lossless: undefined }, true, true)).toBe(page.url);
    expect(comicPageSource({ ...page, preview: undefined }, false, true)).toBe(page.url);
    expect(comicPageSource({ ...page, preview: undefined, lossless: undefined }, true, true)).toBe(page.url);
    expect(comicPageSource(null, true, true)).toBe('');
  });

  it('returns null for unavailable pages', () => {
    expect(comicPageAt(manifest, 2)?.name).toBe('10.png');
    expect(comicPageAt(manifest, 3)).toBeNull();
  });
});
