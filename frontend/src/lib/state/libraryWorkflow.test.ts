import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { FileItem } from '$lib/api/types';
import { ApiClient } from '$lib/api/client';

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
  it('serializes rapid keypresses and wraps across unloaded grid pages', async () => {
    const all = [file('a'), file('b'), file('c'), file('d')];
    const around = vi.spyOn(ApiClient.prototype, 'filesAround').mockImplementation(async (id, _query, _sort, _order, count) => {
      const index = all.findIndex((item) => item.id === id);
      if (index === -1) throw new Error('missing anchor');
      const length = Math.min(count ?? 5, all.length - 1);
      return {
        before: Array.from({ length }, (_, step) => all[(index - step - 1 + all.length) % all.length]),
        after: Array.from({ length }, (_, step) => all[(index + step + 1) % all.length])
      };
    });
    try {
      const library = createLibraryWorkflow();
      // The displayed grid page only contains a, not the other matching files.
      library.openPreview(all[0], [all[0]]);
      await library.movePreview(-1);
      expect(library.activeFile?.id).toBe('d');
      const moves = [library.movePreview(1), library.movePreview(1), library.movePreview(1)];
      await Promise.all(moves);
      expect(library.activeFile?.id).toBe('c');
      expect(around).toHaveBeenCalledWith('a', '', 'added', 'desc', 5);
      expect(clearViewerPreloadCache).toHaveBeenCalledTimes(5);
    } finally {
      around.mockRestore();
    }
  });

  it('navigates a directly opened file without any grid pages and preserves the filter context', async () => {
    const all = [file('first'), file('middle'), file('last')];
    const around = vi.spyOn(ApiClient.prototype, 'filesAround').mockImplementation(async (id, query, _sort, _order, count) => {
      expect(query).toBe('flag:yes');
      const index = all.findIndex((item) => item.id === id);
      const length = Math.min(count ?? 5, all.length - 1);
      return {
        before: Array.from({ length }, (_, step) => all[(index - step - 1 + all.length) % all.length]),
        after: Array.from({ length }, (_, step) => all[(index + step + 1) % all.length])
      };
    });
    try {
      const library = createLibraryWorkflow();
      library.commitSearch('flag:yes');
      library.openPreview(all[1]);
      await library.movePreview(-1);
      expect(library.activeFile?.id).toBe('first');
      await library.movePreview(-1);
      expect(library.activeFile?.id).toBe('last');
      expect(around).toHaveBeenCalledTimes(1);
    } finally {
      around.mockRestore();
    }
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

describe('keyboard range selection', () => {
  it('starts from the cursor anchor, extends, and shrinks from the moving endpoint', () => {
    const library = createLibraryWorkflow();
    const files = [file('a'), file('b'), file('c'), file('d')];

    library.extendSelection(files[0], files[1], files);
    expect(library.selectedCount()).toBe(2);
    expect(files.map((item) => library.isSelected(item.id))).toEqual([true, true, false, false]);

    library.extendSelection(files[1], files[2], files);
    expect(library.selectedCount()).toBe(3);
    expect(files.map((item) => library.isSelected(item.id))).toEqual([true, true, true, false]);

    library.extendSelection(files[2], files[1], files);
    expect(library.selectedCount()).toBe(2);
    expect(files.map((item) => library.isSelected(item.id))).toEqual([true, true, false, false]);

    library.extendSelection(files[1], files[0], files);
    expect(library.selectedCount()).toBe(1);
    expect(files.map((item) => library.isSelected(item.id))).toEqual([true, false, false, false]);
  });
});
