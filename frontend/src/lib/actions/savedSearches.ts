import { errorMessage } from '$lib/utils/format';
import type { FileSort, SortOrder } from '$lib/queries/files';
import type { SavedSearch, SavedSearchRequest } from '$lib/api/types';

interface SavedSearchContext {
  query: string;
  sort: FileSort;
  order: SortOrder;
  create: (body: SavedSearchRequest) => Promise<SavedSearch>;
  update: (id: string, body: SavedSearchRequest) => Promise<SavedSearch>;
  remove: (id: string) => Promise<void>;
}

export async function createSavedSearch(ctx: SavedSearchContext, fallbackName = '') {
  if (!ctx.query) {
    window.alert('Search or choose a kind before saving.');
    return;
  }
  const name = window.prompt('Saved search name', fallbackName || ctx.query);
  if (!name?.trim()) return;
  try {
    await ctx.create({ name: name.trim(), query: ctx.query, sort: ctx.sort, order: ctx.order });
  } catch (error) {
    window.alert(errorMessage(error));
  }
}

export async function updateSavedSearch(ctx: SavedSearchContext, id: string, name: string, previousQuery: string) {
  const nextName = window.prompt('Saved search name', name);
  if (!nextName?.trim()) return;
  try {
    await ctx.update(id, {
      name: nextName.trim(),
      query: ctx.query || previousQuery,
      sort: ctx.sort,
      order: ctx.order
    });
  } catch (error) {
    window.alert(errorMessage(error));
  }
}

export async function deleteSavedSearch(ctx: SavedSearchContext, id: string, name: string) {
  if (!window.confirm(`Delete saved search "${name}"?`)) return;
  try {
    await ctx.remove(id);
  } catch (error) {
    window.alert(errorMessage(error));
  }
}
