import { createInfiniteQuery, createMutation, createQuery } from '@tanstack/svelte-query';
import { ApiClient } from '$lib/api/client';
import { libraryKeys } from './library';
import { jobKeys } from './jobs';
import { offsetPageToken } from '$lib/utils/pagination';
import type { FileListResponse, FileRemovalRequest, FileRemovalResponse, TagMutationOperation, TagMutationRequest, TagMutationResponse } from '$lib/api/types';
import type { QueryClient } from '@tanstack/query-core';
import type { InfiniteData, QueryFunctionContext } from '@tanstack/query-core';

export const pageLimit = 60;
export type FileSort = 'added' | 'modified' | 'name' | 'size' | 'kind';
export type SortOrder = 'asc' | 'desc';

export const fileKeys = {
  all: ['files'] as const,
  pages: (scope: number, query: string, kind: string, sort: FileSort, order: SortOrder, limit: number, paged: boolean, pageIndex: number) =>
    ['files', 'pages', scope, query, kind, sort, order, limit, paged ? `paged:${pageIndex}` : 'infinite'] as const,
  count: (scope: number, query: string) => ['files', 'count', scope, query] as const,
  facets: (scope: number, query: string) => ['files', 'facets', scope, query] as const,
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
  getAuthScope: () => number,
  getEnabled: () => boolean,
  getPageLimit: () => number = () => pageLimit,
  getPaged: () => boolean = () => false,
  getPageIndex: () => number = () => 0
) {
  return createInfiniteQuery<FileListResponse, Error, InfiniteData<FileListResponse, string>, ReturnType<typeof fileKeys.pages>, string>(() => ({
    ...filesQueryOptions(getAuthenticated, getSearch, getKind, getSort, getOrder, getAuthScope, getPageLimit, getPaged, getPageIndex),
    enabled: getAuthenticated() && getEnabled()
  }));
}

export function createFileFacetsQuery(
  getAuthenticated: () => boolean,
  getQuery: () => string,
  getAuthScope: () => number,
  getEnabled: () => boolean = () => true
) {
  return createQuery(() => ({
    queryKey: fileKeys.facets(getAuthScope(), getQuery()),
    enabled: getAuthenticated() && getEnabled(),
    queryFn: ({ signal }) => new ApiClient().listFiles({
      query: getQuery(),
      limit: 1,
      sort: 'added',
      order: 'desc',
      includeFacets: true,
      signal
    })
  }));
}

export function createFileCountQuery(
  getAuthenticated: () => boolean,
  getQuery: () => string,
  getAuthScope: () => number,
  getEnabled: () => boolean = () => true
) {
  return createQuery(() => ({
    queryKey: fileKeys.count(getAuthScope(), getQuery()),
    enabled: getAuthenticated() && getEnabled(),
    // The files API only guarantees an exact total when aggregate metadata is
    // requested. Without it, total_count is deliberately only a pagination
    // lower bound (limit=1 therefore reports at most 2 for larger result sets).
    queryFn: ({ signal }) => new ApiClient().listFiles({
      query: getQuery(),
      limit: 1,
      sort: 'added',
      order: 'desc',
      includeFacets: true,
      signal
    })
  }));
}

export function filesQueryOptions(
  getAuthenticated: () => boolean,
  getSearch: () => string,
  getKind: () => string,
  getSort: () => FileSort,
  getOrder: () => SortOrder,
  getAuthScope: () => number,
  getPageLimit: () => number = () => pageLimit,
  getPaged: () => boolean = () => false,
  getPageIndex: () => number = () => 0
) {
  const limit = getPageLimit();
  const paged = getPaged();
  const pageIndex = paged ? Math.max(0, Math.floor(getPageIndex())) : 0;
  return {
    queryKey: fileKeys.pages(getAuthScope(), getSearch(), getKind(), getSort(), getOrder(), limit, paged, pageIndex),
    enabled: getAuthenticated(),
    initialPageParam: paged ? offsetPageToken(pageIndex, limit) : '',
    queryFn: ({ pageParam, signal }: QueryFunctionContext<ReturnType<typeof fileKeys.pages>, string>) =>
      new ApiClient().listFiles({
        query: queryWithKind(getSearch(), getKind()),
        limit,
        pageToken: pageParam || undefined,
        sort: getSort(),
        order: getOrder(),
        includeFacets: paged || !pageParam,
        signal
      }),
    getNextPageParam: (lastPage: FileListResponse) => lastPage.next_page_token || undefined,
    getPreviousPageParam: (firstPage: FileListResponse) => firstPage.previous_page_token || undefined,
    // Paged mode deliberately retains one transport page. Infinite mode must keep its fetched
    // logical prefix stable because rendering is independently virtualized and eviction can repack
    // already-visible tile geometry.
    ...(paged ? { maxPages: 1 } : {})
  };
}

export interface TagMutationVariables {
  operation: TagMutationOperation;
  body: TagMutationRequest;
}

export function createTagMutation(getCSRFToken: () => string, queryClient: QueryClient) {
  return createMutation<TagMutationResponse, Error, TagMutationVariables>(() => ({
    mutationFn: ({ operation, body }) => new ApiClient(getCSRFToken()).mutateTags(operation, body),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: fileKeys.all }),
        queryClient.invalidateQueries({ queryKey: libraryKeys.tagsRoot })
      ]);
    }
  }));
}

interface RemovalOperation {
  status: 'pending' | 'running' | 'completed' | 'failed' | 'canceled';
  error_message?: string;
}

type DurableFileRemovalResponse = FileRemovalResponse & { operation_id?: string };

async function waitForRemovalOperation(operationID: string) {
  for (;;) {
    const response = await fetch(`/api/v1/operations/${encodeURIComponent(operationID)}`, { credentials: 'same-origin' });
    if (!response.ok) throw new Error(`Unable to read file removal status (HTTP ${response.status}).`);
    const operation = await response.json() as RemovalOperation;
    if (operation.status === 'completed') return;
    if (operation.status === 'failed') throw new Error(operation.error_message || 'File removal failed.');
    if (operation.status === 'canceled') throw new Error('File removal was canceled.');
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
}

async function refreshFileRemovalWhenSettled(operationID: string, queryClient: QueryClient) {
  try {
    await waitForRemovalOperation(operationID);
  } finally {
    // The mutation itself resolves at durable admission so the confirmation
    // dialog can close immediately. Refresh library state only after the
    // background operation reaches a terminal state; failures remain visible in
    // Jobs while this refresh reconciles any partial/no-op result.
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: fileKeys.all }),
      queryClient.invalidateQueries({ queryKey: libraryKeys.tagsRoot }),
      queryClient.invalidateQueries({ queryKey: jobKeys.all })
    ]);
  }
}

export function createFilesRemovalMutation(getCSRFToken: () => string, queryClient: QueryClient) {
  return createMutation<FileRemovalResponse, Error, FileRemovalRequest>(() => ({
    mutationFn: async (body) => {
      // The durable server response confirms admission and supplies operation_id.
      // Do not await terminal filesystem work here: callers use mutateAsync to
      // decide when the confirmation dialog may close.
      const response = await new ApiClient(getCSRFToken()).removeFiles(body) as DurableFileRemovalResponse;
      if (response.operation_id) {
        void refreshFileRemovalWhenSettled(response.operation_id, queryClient).catch(() => undefined);
      }
      return response;
    },
    onSuccess: (response) => {
      const durable = response as DurableFileRemovalResponse;
      // Make a newly admitted durable removal discoverable immediately. The
      // global Jobs query also keeps an idle poll so operations created by any
      // producer cannot disappear between invalidation windows.
      void queryClient.invalidateQueries({ queryKey: jobKeys.all });
      if (!durable.operation_id) {
        void queryClient.invalidateQueries({ queryKey: fileKeys.all });
        void queryClient.invalidateQueries({ queryKey: libraryKeys.tagsRoot });
      }
    }
  }));
}

export interface FileRemovalVariables {
  id: string;
  mode: 'untrack' | 'delete';
}

export function createFileRemovalMutation(getCSRFToken: () => string, queryClient: QueryClient) {
  return createMutation<void, Error, FileRemovalVariables>(() => ({
    mutationFn: ({ id, mode }) => new ApiClient(getCSRFToken()).removeFile(id, mode),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: fileKeys.all }),
        queryClient.invalidateQueries({ queryKey: libraryKeys.tagsRoot })
      ]);
    }
  }));
}
