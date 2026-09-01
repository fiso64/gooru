import { createInfiniteQuery, createMutation } from '@tanstack/svelte-query';
import { ApiClient } from '$lib/api/client';
import { libraryKeys } from './library';
import type { FileListResponse, TagMutationOperation, TagMutationRequest, TagMutationResponse } from '$lib/api/types';
import type { QueryClient } from '@tanstack/query-core';
import type { InfiniteData, QueryFunctionContext } from '@tanstack/query-core';

export const pageLimit = 60;
export const retainedFilePages = 8;
export type FileSort = 'modified' | 'name' | 'size' | 'kind';
export type SortOrder = 'asc' | 'desc';

export const fileKeys = {
  all: ['files'] as const,
  pages: (scope: number, query: string, kind: string, sort: FileSort, order: SortOrder) =>
    ['files', 'pages', scope, query, kind, sort, order] as const,
  suggestions: (scope: number, q: string, existing: string) => ['files', 'suggestions', scope, q, existing] as const
};

function queryWithKind(search: string, kind: string) {
  const parts = [search.trim()];
  if (kind) parts.push(`type:${kind}`);
  return parts.filter(Boolean).join(' ');
}

export function pageTokenOffset(token: string | undefined) {
  if (!token) return 0;
  if (/^\d+$/.test(token)) return Number(token);
  try {
    const decoded = atob(token.replace(/-/g, '+').replace(/_/g, '/'));
    const match = decoded.match(/^offset:(\d+)$/);
    return match ? Number(match[1]) : 0;
  } catch {
    return 0;
  }
}

export function createFilesQuery(
  getAuthenticated: () => boolean,
  getSearch: () => string,
  getKind: () => string,
  getSort: () => FileSort,
  getOrder: () => SortOrder,
  getAuthScope: () => number
) {
  return createInfiniteQuery<FileListResponse, Error, InfiniteData<FileListResponse, string>, ReturnType<typeof fileKeys.pages>, string>(() =>
    filesQueryOptions(getAuthenticated, getSearch, getKind, getSort, getOrder, getAuthScope)
  );
}

export function filesQueryOptions(
  getAuthenticated: () => boolean,
  getSearch: () => string,
  getKind: () => string,
  getSort: () => FileSort,
  getOrder: () => SortOrder,
  getAuthScope: () => number
) {
  return {
    queryKey: fileKeys.pages(getAuthScope(), getSearch(), getKind(), getSort(), getOrder()),
    enabled: getAuthenticated(),
    initialPageParam: '',
    queryFn: ({ pageParam, signal }: QueryFunctionContext<ReturnType<typeof fileKeys.pages>, string>) =>
      new ApiClient().listFiles({
        query: queryWithKind(getSearch(), getKind()),
        limit: pageLimit,
        pageToken: pageParam || undefined,
        sort: getSort(),
        order: getOrder(),
        includeFacets: !pageParam,
        signal
      }),
    getNextPageParam: (lastPage: FileListResponse) => lastPage.next_page_token || undefined,
    getPreviousPageParam: (firstPage: FileListResponse) => firstPage.previous_page_token || undefined,
    maxPages: retainedFilePages,
    placeholderData: (previousData: InfiniteData<FileListResponse, string> | undefined) => previousData
  };
}

export interface TagMutationVariables {
  operation: TagMutationOperation;
  body: TagMutationRequest;
}

export function createTagMutation(getCSRFToken: () => string, queryClient: QueryClient) {
  return createMutation<TagMutationResponse, Error, TagMutationVariables>(() => ({
    mutationFn: ({ operation, body }) => new ApiClient(getCSRFToken()).mutateTags(operation, body),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: fileKeys.all });
      void queryClient.invalidateQueries({ queryKey: libraryKeys.tags });
    }
  }));
}

export function createUntrackFileMutation(getCSRFToken: () => string, queryClient: QueryClient) {
  return createMutation<void, Error, string>(() => ({
    mutationFn: (id) => new ApiClient(getCSRFToken()).untrackFile(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: fileKeys.all });
      void queryClient.invalidateQueries({ queryKey: libraryKeys.tags });
    }
  }));
}
