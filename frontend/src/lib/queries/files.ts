import { createInfiniteQuery } from '@tanstack/svelte-query';
import { ApiClient } from '$lib/api/client';
import type { FileListResponse } from '$lib/api/types';

export const pageLimit = 36;

export function createFilesQuery(getToken: () => string, getSearch: () => string) {
  return createInfiniteQuery<FileListResponse, Error, { pages: FileListResponse[]; pageParams: string[] }, [string, string], string>(() => ({
    queryKey: ['files', getSearch()],
    enabled: Boolean(getToken()),
    initialPageParam: '',
    queryFn: ({ pageParam }) =>
      new ApiClient(getToken()).listFiles({
        query: getSearch(),
        limit: pageLimit,
        pageToken: pageParam || undefined
      }),
    getNextPageParam: (lastPage) => lastPage.next_page_token || undefined
  }));
}
