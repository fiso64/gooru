import { createMutation, createQuery } from '@tanstack/svelte-query';
import { ApiClient } from '$lib/api/client';
import { uploadSegmentedFiles } from '$lib/api/segmentedUploads';
import type { SavedSearch, SavedSearchRequest, UploadImportResponse } from '$lib/api/types';
import type { BackgroundOperation } from '$lib/api/operations';
import type { UploadAddedAtStrategy } from '$lib/state/uploadItems';
import type { QueryClient } from '@tanstack/query-core';

export const libraryKeys = {
  root: ['library'] as const,
  tagsRoot: ['library', 'tags'] as const,
  savedSearchesRoot: ['library', 'saved-searches'] as const,
  savedSearches: (scope: number) => ['library', 'saved-searches', scope] as const,
  tags: (scope: number) => ['library', 'tags', 'common', scope] as const,
  allTags: (username: string) => ['library', 'tags', 'all', username] as const,
  uploadTargets: (scope: number) => ['library', 'upload-targets', scope] as const,
  suggestionsRoot: ['library', 'suggestions'] as const,
  suggestions: (scope: number | string, q: string, existing: string) => ['library', 'suggestions', scope, q, existing] as const
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
    queryFn: () => new ApiClient().listTags(true, 20)
  }));
}

export function createAllTagsQuery(getAuthenticated: () => boolean, getUsername: () => string) {
  return createQuery(() => ({
    queryKey: libraryKeys.allTags(getUsername()),
    enabled: getAuthenticated(),
    queryFn: () => new ApiClient().listTags(true, 0)
  }));
}

export function createSuggestionsQuery(
  getAuthenticated: () => boolean,
  getDraft: () => string,
  getExisting: () => string,
  getAuthScope: () => number | string
) {
  return createQuery(() => {
    const scope = getAuthScope();
    const draft = getDraft().trim();
    const existing = getExisting().trim();
    return {
      queryKey: libraryKeys.suggestions(scope, draft, existing),
      enabled: getAuthenticated() && draft.length > 0,
      queryFn: ({ signal }) => new ApiClient().searchSuggestions(draft, 10, existing, signal),
      staleTime: 30_000,
      placeholderData: (previousData, previousQuery) =>
        getAuthenticated() && previousQuery?.queryKey[2] === scope && previousQuery?.queryKey[4] === existing
          ? previousData : undefined
    };
  });
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
  itemTags?: string[][];
  preferAsync: boolean;
  targetID: string;
  conflictPolicy: string;
  addedAtStrategy: UploadAddedAtStrategy;
  queueTimeMs: number[];
  queueFirstTimeMs: number;
  queueLastTimeMs: number;
  queueIndex: number[];
  queueTotal: number[];
  operationID?: string;
  segmentIndex?: number;
  segmentCount?: number;
  onProgress?: (progress: number) => void;
  signal?: AbortSignal;
}

export function createUploadMutation(getCSRFToken: () => string) {
  return createMutation<BackgroundOperation | UploadImportResponse, Error, UploadVariables>(() => ({
    mutationFn: ({ files, tags, itemTags, preferAsync, targetID, conflictPolicy, addedAtStrategy, queueTimeMs, queueFirstTimeMs, queueLastTimeMs, queueIndex, queueTotal, operationID, segmentIndex, segmentCount, onProgress, signal }) => {
      if (preferAsync && segmentCount !== undefined && segmentCount > 1 && segmentIndex !== undefined) {
        return uploadSegmentedFiles(getCSRFToken(), {
          files,
          tags,
          itemTags,
          targetID,
          conflictPolicy,
          addedAtStrategy,
          queueTimeMs,
          queueFirstTimeMs,
          queueLastTimeMs,
          queueIndex,
          queueTotal,
          operationID,
          segmentIndex,
          segmentCount,
          onProgress,
          signal
        });
      }
      return new ApiClient(getCSRFToken()).uploadFiles(files, tags, preferAsync, targetID, conflictPolicy, onProgress, {
        addedAtStrategy,
        queueTimeMs,
        queueFirstTimeMs,
        queueLastTimeMs,
        queueIndex,
        queueTotal,
        itemTags,
        signal
      });
    }
  }));
}

export async function refreshUploadQueries(queryClient: QueryClient) {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ['files'] }),
    queryClient.invalidateQueries({ queryKey: libraryKeys.tagsRoot }),
    queryClient.invalidateQueries({ queryKey: libraryKeys.suggestionsRoot })
  ]);
}
