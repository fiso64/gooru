import { createMutation, createQuery } from '@tanstack/svelte-query';
import { ApiClient } from '$lib/api/client';
import type { SavedSearch, SavedSearchRequest, UploadImportResponse, Job } from '$lib/api/types';
import type { UploadAddedAtStrategy } from '$lib/state/uploadItems';
import type { QueryClient } from '@tanstack/query-core';

export const libraryKeys = {
  root: ['library'] as const,
  tagsRoot: ['library', 'tags'] as const,
  savedSearchesRoot: ['library', 'saved-searches'] as const,
  savedSearches: (scope: number) => ['library', 'saved-searches', scope] as const,
  tags: (scope: number) => ['library', 'tags', scope] as const,
  uploadTargets: (scope: number) => ['library', 'upload-targets', scope] as const,
  suggestions: (scope: number, q: string) => ['library', 'suggestions', scope, q] as const
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
    queryKey: libraryKeys.suggestions(getAuthScope(), getDraft()),
    enabled: getAuthenticated() && getDraft().trim().length > 0,
    queryFn: ({ signal }) => new ApiClient().searchSuggestions(getDraft().trim(), 10, getExisting(), signal),
    staleTime: 30_000
  }));
}

export interface SavedSearchCreateVariables extends SavedSearchRequest {}

export interface SavedSearchUpdateVariables {
  id: string;
  body: SavedSearchRequest;
}

export function createSavedSearchCreateMutation(getCSRFToken: () => string, queryClient: QueryClient) {
  return createMutation<SavedSearch, Error, SavedSearchCreateVariables>(() => ({
    mutationFn: (body) => new ApiClient(getCSRFToken()).createSavedSearch(body),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: libraryKeys.savedSearchesRoot })
  }));
}

export function createSavedSearchUpdateMutation(getCSRFToken: () => string, queryClient: QueryClient) {
  return createMutation<SavedSearch, Error, SavedSearchUpdateVariables>(() => ({
    mutationFn: ({ id, body }) => new ApiClient(getCSRFToken()).updateSavedSearch(id, body),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: libraryKeys.savedSearchesRoot })
  }));
}

export function createSavedSearchDeleteMutation(getCSRFToken: () => string, queryClient: QueryClient) {
  return createMutation<void, Error, string>(() => ({
    mutationFn: (id) => new ApiClient(getCSRFToken()).deleteSavedSearch(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: libraryKeys.savedSearchesRoot })
  }));
}

export interface UploadVariables {
  files: File[];
  tags: string[];
  preferAsync: boolean;
  targetID: string;
  conflictPolicy: string;
  addedAtStrategy: UploadAddedAtStrategy;
  queueTimeMs: number;
  queueFirstTimeMs: number;
  queueLastTimeMs: number;
  queueIndex: number;
  queueTotal: number;
  onProgress?: (progress: number) => void;
}

export function createUploadMutation(getCSRFToken: () => string, queryClient: QueryClient) {
  return createMutation<Job | UploadImportResponse, Error, UploadVariables>(() => ({
    mutationFn: ({ files, tags, preferAsync, targetID, conflictPolicy, addedAtStrategy, queueTimeMs, queueFirstTimeMs, queueLastTimeMs, queueIndex, queueTotal, onProgress }) =>
      new ApiClient(getCSRFToken()).uploadFiles(files, tags, preferAsync, targetID, conflictPolicy, onProgress, {
        addedAtStrategy,
        queueTimeMs: [queueTimeMs],
        queueFirstTimeMs,
        queueLastTimeMs,
        queueIndex: [queueIndex],
        queueTotal: [queueTotal]
      }),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['files'] }),
        queryClient.invalidateQueries({ queryKey: ['jobs'] }),
        queryClient.invalidateQueries({ queryKey: libraryKeys.tagsRoot })
      ]);
    }
  }));
}
