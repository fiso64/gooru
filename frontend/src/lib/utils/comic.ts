import type { ComicManifest, ComicPage, FileItem } from '$lib/api/types';

const comicMimeType = 'application/vnd.comicbook+zip';

export function isComicFile(file: Pick<FileItem, 'name' | 'media_type'>): boolean {
  return file.media_type.trim().toLowerCase() === comicMimeType || file.name.trim().toLowerCase().endsWith('.cbz');
}

export function comicPageAt(manifest: ComicManifest | null, index: number): ComicPage | null {
  if (!manifest || index < 0 || index >= manifest.pages.length) return null;
  return manifest.pages[index] ?? null;
}

export function comicPageSource(page: ComicPage | null, preferOriginal: boolean, preferLossless = true): string {
  if (!page) return '';
  if (!preferOriginal && page.preview) return page.preview;
  return preferLossless && page.lossless ? page.lossless : page.url;
}

export function moveComicPage(index: number, delta: number, pageCount: number): number {
  if (pageCount <= 0) return 0;
  return Math.min(pageCount - 1, Math.max(0, index + delta));
}

export function adjacentComicPages(manifest: ComicManifest | null, index: number): ComicPage[] {
  if (!manifest) return [];
  return [comicPageAt(manifest, index - 1), comicPageAt(manifest, index + 1)].filter((page): page is ComicPage => Boolean(page));
}
