import { describe, expect, it, vi } from 'vitest';
import { QueryClient } from '@tanstack/query-core';
import { filesQueryOptions, pageLimit, pageTokenOffset } from './files';

describe('files query options', () => {
  it('keeps loaded pages coherent and only requests facets for the first page', async () => {
    const requests: Array<{ url: string; signal?: AbortSignal }> = [];
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      requests.push({ url: `${new URL(request.url).pathname}${new URL(request.url).search}`, signal: request.signal });
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

    // Infinite scrolling virtualizes rendered media independently. Query-level page eviction
    // would mutate the logical prefix under the viewport and can repack already-visible tiles.
    expect('maxPages' in options).toBe(false);
    await options.queryFn({ client, pageParam: '', signal, queryKey: options.queryKey, direction: 'forward', meta: undefined });
    await options.queryFn({ client, pageParam: 'next-page', signal, queryKey: options.queryKey, direction: 'forward', meta: undefined });

    expect(requests).toHaveLength(2);
    expect(requests[0].signal).toBeInstanceOf(AbortSignal);
    expect(requests[0].signal?.aborted).toBe(false);
    expect(requests[0].url).toContain(`limit=${pageLimit}`);
    expect(new URL(requests[0].url, 'http://localhost').searchParams.get('query')).toBe('rating:safe type:photo');
    expect(requests[0].url).toContain('include_facets=true');
    expect(requests[1].url).toContain('page_token=next-page');
    expect(requests[1].url).not.toContain('include_facets=true');
  });

  it('does not expose placeholder rows for a changed library query', () => {
    const options = filesQueryOptions(
      () => true,
      () => '',
      () => '',
      () => 'modified',
      () => 'desc',
      () => 1
    );

    expect('placeholderData' in options).toBe(false);
  });

  it('decodes offset page tokens for retained window positioning', () => {
    expect(pageTokenOffset('')).toBe(0);
    expect(pageTokenOffset('180')).toBe(180);
    expect(pageTokenOffset(btoa('offset:240').replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, ''))).toBe(240);
  });
});
