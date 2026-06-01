import { createInfiniteQuery } from '@tanstack/svelte-query';
import { ApiClient } from '$lib/api/client';
import type { FileListResponse } from '$lib/api/types';

export const pageLimit = 60;
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
  if (kind) parts.push(`kind:${kind}`);
  return parts.filter(Boolean).join(' ');
}

export function createFilesQuery(
  getAuthenticated: () => boolean,
  getSearch: () => string,
  getKind: () => string,
  getSort: () => FileSort,
  getOrder: () => SortOrder,
  getAuthScope: () => number
) {
  return createInfiniteQuery<FileListResponse, Error, { pages: FileListResponse[]; pageParams: string[] }, ReturnType<typeof fileKeys.pages>, string>(() => ({
    queryKey: fileKeys.pages(getAuthScope(), getSearch(), getKind(), getSort(), getOrder()),
    enabled: getAuthenticated(),
    initialPageParam: '',
    queryFn: ({ pageParam, signal }) =>
      new ApiClient().listFiles({
        query: queryWithKind(getSearch(), getKind()),
        limit: pageLimit,
        pageToken: pageParam || undefined,
        sort: getSort(),
        order: getOrder(),
        includeFacets: !pageParam,
        signal
      }),
    getNextPageParam: (lastPage) => lastPage.next_page_token || undefined
  }));
}
