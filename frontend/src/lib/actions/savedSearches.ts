import { ApiClient } from '$lib/api/client';
import { errorMessage } from '$lib/utils/format';
import type { FileSort, SortOrder } from '$lib/queries/files';

interface SavedSearchContext {
  csrfToken: string;
  query: string;
  sort: FileSort;
  order: SortOrder;
  refetch: () => Promise<unknown>;
}

export async function createSavedSearch(ctx: SavedSearchContext, fallbackName = '') {
  if (!ctx.query) {
    window.alert('Search or choose a kind before saving.');
    return;
  }
  const name = window.prompt('Saved search name', fallbackName || ctx.query);
  if (!name?.trim()) return;
  try {
    await new ApiClient(ctx.csrfToken).createSavedSearch({ name: name.trim(), query: ctx.query, sort: ctx.sort, order: ctx.order });
    await ctx.refetch();
  } catch (error) {
    window.alert(errorMessage(error));
  }
}

export async function updateSavedSearch(ctx: SavedSearchContext, id: string, name: string, previousQuery: string) {
  const nextName = window.prompt('Saved search name', name);
  if (!nextName?.trim()) return;
  try {
    await new ApiClient(ctx.csrfToken).updateSavedSearch(id, {
      name: nextName.trim(),
      query: ctx.query || previousQuery,
      sort: ctx.sort,
      order: ctx.order
    });
    await ctx.refetch();
  } catch (error) {
    window.alert(errorMessage(error));
  }
}

export async function deleteSavedSearch(csrfToken: string, id: string, name: string, refetch: () => Promise<unknown>) {
  if (!window.confirm(`Delete saved search "${name}"?`)) return;
  try {
    await new ApiClient(csrfToken).deleteSavedSearch(id);
    await refetch();
  } catch (error) {
    window.alert(errorMessage(error));
  }
}
