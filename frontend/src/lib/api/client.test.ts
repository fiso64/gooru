import { describe, expect, it } from 'vitest';
import { ApiClient, ApiError } from './client';

describe('ApiClient', () => {
  it('sends bearer auth and query parameters', async () => {
    const requests: Array<{ url: string; headers: Headers }> = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      requests.push({
        url: input.toString(),
        headers: new Headers(init?.headers)
      });
      return Response.json({ files: [] });
    }) as typeof fetch;

    const client = new ApiClient('secret-token');
    await client.listFiles({ query: 'kind:image', limit: 24, pageToken: 'next' });

    expect(requests).toHaveLength(1);
    expect(requests[0].headers.get('Authorization')).toBe('Bearer secret-token');
    expect(requests[0].url).toContain('/api/v1/files?query=kind%3Aimage&limit=24&page_token=next');
  });

  it('maps API error envelopes', async () => {
    globalThis.fetch = (async () =>
      Response.json({ error: { code: 'unauthorized', message: 'missing bearer token' } }, { status: 401 })) as typeof fetch;

    const client = new ApiClient('bad-token');
    await expect(client.listFiles()).rejects.toMatchObject({
      status: 401,
      code: 'unauthorized',
      message: 'missing bearer token'
    });
  });
});
