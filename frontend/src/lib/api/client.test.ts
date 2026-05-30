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

  it('sends tag mutation requests with bearer auth', async () => {
    const requests: Array<{ url: string; method?: string; headers: Headers; body?: BodyInit | null }> = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      requests.push({
        url: input.toString(),
        method: init?.method,
        headers: new Headers(init?.headers),
        body: init?.body
      });
      return Response.json({ operation: 'add', selector: { file_ids: ['file-one'] }, affected_count: 1 });
    }) as typeof fetch;

    const client = new ApiClient('secret-token', '/custom-api');
    await client.mutateTags('add', { file_ids: ['file-one'], tags: ['reviewed'] });

    expect(requests).toHaveLength(1);
    expect(requests[0].url).toBe('/custom-api/files/tags');
    expect(requests[0].method).toBe('POST');
    expect(requests[0].headers.get('Authorization')).toBe('Bearer secret-token');
    expect(requests[0].headers.get('Content-Type')).toBe('application/json');
    expect(requests[0].body).toBe(JSON.stringify({ file_ids: ['file-one'], tags: ['reviewed'] }));
  });

  it('uploads files with async preference', async () => {
    const requests: Array<{ url: string; method?: string; headers: Headers; body?: BodyInit | null }> = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      requests.push({
        url: input.toString(),
        method: init?.method,
        headers: new Headers(init?.headers),
        body: init?.body
      });
      return Response.json({ id: 'job-one', type: 'upload_import', status: 'pending' }, { status: 202 });
    }) as typeof fetch;

    const client = new ApiClient('secret-token');
    const file = new File(['hello'], 'hello.txt', { type: 'text/plain' });
    await client.uploadFiles([file], ['reviewed']);

    expect(requests).toHaveLength(1);
    expect(requests[0].url).toBe('/api/v1/uploads');
    expect(requests[0].method).toBe('POST');
    expect(requests[0].headers.get('Authorization')).toBe('Bearer secret-token');
    expect(requests[0].headers.get('Prefer')).toBe('respond-async');
    expect(requests[0].body).toBeInstanceOf(FormData);
  });

  it('fetches and cancels jobs with bearer auth', async () => {
    const requests: Array<{ url: string; method?: string; headers: Headers }> = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      requests.push({
        url: input.toString(),
        method: init?.method,
        headers: new Headers(init?.headers)
      });
      return Response.json({ id: 'job-one', type: 'upload_import', status: 'canceled' });
    }) as typeof fetch;

    const client = new ApiClient('secret-token');
    await client.getJob('job-one');
    await client.cancelJob('job-one');

    expect(requests).toHaveLength(2);
    expect(requests[0].url).toBe('/api/v1/jobs/job-one');
    expect(requests[0].method).toBeUndefined();
    expect(requests[0].headers.get('Authorization')).toBe('Bearer secret-token');
    expect(requests[1].url).toBe('/api/v1/jobs/job-one');
    expect(requests[1].method).toBe('DELETE');
    expect(requests[1].headers.get('Authorization')).toBe('Bearer secret-token');
  });
});
