import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { FileItem } from '$lib/api/types';

const { preloadViewerMedia } = vi.hoisted(() => ({ preloadViewerMedia: vi.fn(() => Promise.resolve()) }));
vi.mock('$lib/utils/viewerPreload', () => ({
  preloadViewerMedia,
  clearViewerPreloadCache: vi.fn()
}));

import { createLibraryWorkflow } from './libraryWorkflow.svelte';

function storeValue<T>(store: { subscribe: (run: (value: T) => void) => () => void }): T {
  let value!: T;
  const unsubscribe = store.subscribe((next) => (value = next));
  unsubscribe();
  return value;
}

function file(id: string): FileItem {
  return {
    id,
    media_kind: 'image',
    media_type: 'image/jpeg',
    media_urls: { content: `/content/${id}`, preview: `/preview/${id}`, thumbnail: '', download: '' }
  } as FileItem;
}

beforeEach(() => preloadViewerMedia.mockClear());

describe('tag search transition', () => {
  it('uses the canonical tag search state and closes the preview', () => {
    const library = createLibraryWorkflow();
    library.setKind('video');
    library.openPreview({ id: 'file-1' } as FileItem);

    library.runTagSearch('artist:example');

    expect(library.activeFile).toBeNull();
    expect(library.activeKind).toBe('');
    expect(library.route).toBe('library');
    expect(storeValue(library.searchDraft)).toBe('artist:example');
    expect(storeValue(library.submittedSearch)).toBe('artist:example');
  });
});

describe('preview navigation', () => {
  it('primes the next item and remains deterministic during rapid right navigation', () => {
    const library = createLibraryWorkflow();
    const files = [file('a'), file('b'), file('c'), file('d')];

    library.openPreview(files[0], files);
    library.movePreview(1, files);
    library.movePreview(1, files);
    library.movePreview(1, files);

    expect(library.activeFile?.id).toBe('d');
    expect(preloadViewerMedia).toHaveBeenCalledWith(files[1]);
    expect(preloadViewerMedia).toHaveBeenCalledWith(files[2]);
    expect(preloadViewerMedia).toHaveBeenCalledWith(files[3]);
    expect(preloadViewerMedia).toHaveBeenCalledWith(files[0]);
  });
});
