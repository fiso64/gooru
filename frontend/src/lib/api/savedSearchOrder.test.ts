import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiClient, setUnauthorizedHandler } from './client';

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
  setUnauthorizedHandler(undefined);
});

describe('saved search reorder API', () => {
  it('sends the complete order with PUT and CSRF', async () => {
    const requests: Array<{ url: string; method: string; headers: Headers; body: string }> = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      requests.push({
        url: new URL(request.url).pathname,
        method: request.method,
        headers: request.headers,
        body: await request.clone().text()
      });
      return Response.json({ ok: true });
    }) as typeof fetch;

    await new ApiClient('secret-token').reorderSavedSearches(['saved-three', 'saved-one', 'saved-two']);

    expect(requests).toHaveLength(1);
    expect(requests[0].url).toBe('/api/v1/saved-searches/reorder');
    expect(requests[0].method).toBe('PUT');
    expect(requests[0].headers.get('X-Gooru-CSRF')).toBe('secret-token');
    expect(requests[0].headers.get('Content-Type')).toBe('application/json');
    expect(JSON.parse(requests[0].body)).toEqual({ ids: ['saved-three', 'saved-one', 'saved-two'] });
  });

  it('maps reorder errors and notifies the unauthorized handler', async () => {
    const unauthorized = vi.fn();
    setUnauthorizedHandler(unauthorized);
    globalThis.fetch = (async () =>
      Response.json({ error: { code: 'unauthorized', message: 'session expired' } }, { status: 401 })) as typeof fetch;

    await expect(new ApiClient('expired').reorderSavedSearches(['saved-one'])).rejects.toMatchObject({
      status: 401,
      code: 'unauthorized',
      message: 'session expired'
    });
    expect(unauthorized).toHaveBeenCalledOnce();
  });
});
