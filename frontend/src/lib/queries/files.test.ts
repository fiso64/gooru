import { describe, expect, it, vi } from 'vitest';
import { QueryClient } from '@tanstack/query-core';
import { filesQueryOptions, pageLimit } from './files';

describe('files query options', () => {
  it('keeps loaded pages coherent and only requests facets for the first page', async () => {
    const requests: Array<{ url: string; signal?: AbortSignal }> = [];
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      requests.push({ url: input.toString(), signal: init?.signal ?? undefined });
      return Response.json({ files: [], next_page_token: '', total_count: 0, library_count: 0 });
    }) as typeof fetch;

    const options = filesQueryOptions(
      () => true,
      () => 'rating:safe',
      () => 'photo',
      () => 'modified',
      () => 'desc',
      () => 7
    );
    const signal = new AbortController().signal;
    const client = new QueryClient();

    expect('maxPages' in options).toBe(false);
    await options.queryFn({ client, pageParam: '', signal, queryKey: options.queryKey, direction: 'forward', meta: undefined });
    await options.queryFn({ client, pageParam: 'next-page', signal, queryKey: options.queryKey, direction: 'forward', meta: undefined });

    expect(requests).toHaveLength(2);
    expect(requests[0].signal).toBe(signal);
    expect(requests[0].url).toContain(`limit=${pageLimit}`);
    expect(requests[0].url).toContain('query=rating%3Asafe+kind%3Aphoto');
    expect(requests[0].url).toContain('include_facets=true');
    expect(requests[1].url).toContain('page_token=next-page');
    expect(requests[1].url).not.toContain('include_facets=true');
  });
});
