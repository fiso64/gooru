import { createQuery } from '@tanstack/svelte-query';
import { ApiClient } from '$lib/api/client';

export const libraryKeys = {
  savedSearches: (scope: number) => ['library', 'saved-searches', scope] as const,
  tags: (scope: number) => ['library', 'tags', scope] as const,
  uploadTargets: (scope: number) => ['library', 'upload-targets', scope] as const,
  suggestions: (scope: number, q: string, existing: string) => ['library', 'suggestions', scope, q, existing] as const
};

export function createSavedSearchesQuery(getAuthenticated: () => boolean, getAuthScope: () => number) {
  return createQuery(() => ({
    queryKey: libraryKeys.savedSearches(getAuthScope()),
    enabled: getAuthenticated(),
    queryFn: () => new ApiClient().listSavedSearches()
  }));
}

export function createUploadTargetsQuery(getAuthenticated: () => boolean, getAuthScope: () => number) {
  return createQuery(() => ({
    queryKey: libraryKeys.uploadTargets(getAuthScope()),
    enabled: getAuthenticated(),
    queryFn: () => new ApiClient().getUploadTargets()
  }));
}

export function createTagsQuery(getAuthenticated: () => boolean, getAuthScope: () => number) {
  return createQuery(() => ({
    queryKey: libraryKeys.tags(getAuthScope()),
    enabled: getAuthenticated(),
    queryFn: () => new ApiClient().listTags(true)
  }));
}

export function createSuggestionsQuery(
  getAuthenticated: () => boolean,
  getDraft: () => string,
  getExisting: () => string,
  getAuthScope: () => number
) {
  return createQuery(() => ({
    queryKey: libraryKeys.suggestions(getAuthScope(), getDraft(), getExisting()),
    enabled: getAuthenticated() && getDraft().trim().length > 0,
    queryFn: ({ signal }) => new ApiClient().searchSuggestions(getDraft().trim(), 10, getExisting(), signal),
    staleTime: 30_000
  }));
}
