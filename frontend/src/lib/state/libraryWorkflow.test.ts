import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { FileItem } from '$lib/api/types';

const { clearViewerPreloadCache } = vi.hoisted(() => ({ clearViewerPreloadCache: vi.fn() }));
vi.mock('$lib/utils/viewerPreload', () => ({
  clearViewerPreloadCache
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
    media_kind: 'photo',
    media_type: 'image/jpeg',
    media_urls: { content: `/content/${id}`, preview: `/preview/${id}`, thumbnail: '', download: '' }
  } as unknown as FileItem;
}

beforeEach(() => clearViewerPreloadCache.mockClear());

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
  it('cancels speculative preload work while remaining deterministic during rapid right navigation', () => {
    const library = createLibraryWorkflow();
    const files = [file('a'), file('b'), file('c'), file('d')];

    library.openPreview(files[0], files);
    library.movePreview(1, files);
    library.movePreview(1, files);
    library.movePreview(1, files);

    expect(library.activeFile?.id).toBe('d');
    expect(clearViewerPreloadCache).toHaveBeenCalledTimes(4);
  });
});


describe('completion responsiveness', () => {
  it('publishes suggestion drafts immediately while keeping library submission debounced', () => {
    vi.useFakeTimers();
    try {
      const library = createLibraryWorkflow();
      library.setSearchDraft('animal');
      expect(storeValue(library.suggestionSearch)).toBe('animal');

      library.setSearch('animal:cat');
      expect(storeValue(library.suggestionSearch)).toBe('animal:cat');
      expect(storeValue(library.submittedSearch)).toBe('');
      vi.advanceTimersByTime(280);
      expect(storeValue(library.submittedSearch)).toBe('animal:cat');
    } finally {
      vi.useRealTimers();
    }
  });
});
