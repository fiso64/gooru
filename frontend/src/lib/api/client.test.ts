import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiClient, ApiError, setUnauthorizedHandler } from './client';

const originalXMLHttpRequest = globalThis.XMLHttpRequest;

afterEach(() => {
  globalThis.XMLHttpRequest = originalXMLHttpRequest;
});

describe('ApiClient', () => {
  it('sends cookie-authenticated query parameters', async () => {
    const requests: Array<{ url: string; headers: Headers; credentials?: RequestCredentials }> = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      requests.push({
        url: relativeURL(request.url),
        headers: request.headers,
        credentials: request.credentials
      });
      return Response.json({ files: [] });
    }) as typeof fetch;

    const client = new ApiClient();
    await client.listFiles({ query: 'kind:image', limit: 24, pageToken: 'next' });

    expect(requests).toHaveLength(1);
    expect(requests[0].headers.get('Authorization')).toBeNull();
    expect(requests[0].credentials).toBe('same-origin');
    expect(requests[0].url).toContain('/api/v1/files?query=kind%3Aimage&limit=24&page_token=next');
  });

  it('uses added-desc defaults when creating a saved search without explicit sorting', async () => {
    let requestBody = '';
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      requestBody = await request.clone().text();
      return Response.json({ id: 'saved-one', name: 'Recent', query: '', sort: 'added', order: 'desc', created_at: '', updated_at: '' });
    }) as typeof fetch;

    await new ApiClient('csrf').createSavedSearch({ name: 'Recent', query: '' });

    expect(JSON.parse(requestBody)).toMatchObject({ name: 'Recent', query: '', sort: 'added', order: 'desc' });
  });

  it('maps API error envelopes', async () => {
    globalThis.fetch = (async () =>
      Response.json({ error: { code: 'unauthorized', message: 'login required' } }, { status: 401 })) as typeof fetch;

    const client = new ApiClient();
    await expect(client.listFiles()).rejects.toMatchObject({
      status: 401,
      code: 'unauthorized',
      message: 'login required'
    });
  });

  it('notifies the central unauthorized handler on 401 responses', async () => {
    const unauthorized = vi.fn();
    setUnauthorizedHandler(unauthorized);
    globalThis.fetch = (async () =>
      Response.json({ error: { code: 'unauthorized', message: 'session expired' } }, { status: 401 })) as typeof fetch;

    await expect(new ApiClient().listFiles()).rejects.toMatchObject({ status: 401 });
    expect(unauthorized).toHaveBeenCalledOnce();
    setUnauthorizedHandler(undefined);
  });

  it('sends tag mutation requests with CSRF', async () => {
    const requests: Array<{ url: string; method?: string; headers: Headers; body?: BodyInit | null }> = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      requests.push({
        url: relativeURL(request.url),
        method: request.method === 'GET' ? undefined : request.method,
        headers: request.headers,
        body: request.body
      });
      return Response.json({ operation: 'add', selector: { file_ids: ['file-one'] }, affected_count: 1 });
    }) as typeof fetch;

    const client = new ApiClient('secret-token', '/custom-api');
    await client.mutateTags('add', { file_ids: ['file-one'], tags: ['reviewed'] });

    expect(requests).toHaveLength(1);
    expect(requests[0].url).toBe('/custom-api/files/tags');
    expect(requests[0].method).toBe('POST');
    expect(requests[0].headers.get('Authorization')).toBeNull();
    expect(requests[0].headers.get('X-Gooru-CSRF')).toBe('secret-token');
    expect(requests[0].headers.get('Content-Type')).toBe('application/json');
    expect(JSON.parse(await bodyText(requests[0].body))).toEqual({ file_ids: ['file-one'], tags: ['reviewed'], verbose: false });
  });

  it('changes passwords with CSRF and generated request fields', async () => {
    const requests: Array<{ url: string; method: string; headers: Headers; body: string }> = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      requests.push({
        url: relativeURL(request.url),
        method: request.method,
        headers: request.headers,
        body: await request.clone().text()
      });
      return Response.json({ ok: true });
    }) as typeof fetch;

    await new ApiClient('secret-token').changePassword('old-secret', 'new-secret');

    expect(requests).toHaveLength(1);
    expect(requests[0].url).toBe('/api/v1/auth/change-password');
    expect(requests[0].method).toBe('POST');
    expect(requests[0].headers.get('Authorization')).toBeNull();
    expect(requests[0].headers.get('X-Gooru-CSRF')).toBe('secret-token');
    expect(JSON.parse(requests[0].body)).toEqual({ current_password: 'old-secret', new_password: 'new-secret' });
  });

  it('uploads multipart files through progress-capable browser transport', async () => {
    const xhr = installUploadXHR({
      status: 202,
      response: { id: 'job-one', type: 'upload_import', status: 'pending' },
      progress: [[2, 10], [7, 10], [10, 10]]
    });
    const progress = vi.fn();
    const client = new ApiClient('secret-token');
    const file = new File(['hello'], 'hello.txt', { type: 'text/plain' });

    const result = await client.uploadFiles([file], ['reviewed'], true, '', 'rename', progress);

    expect(result).toMatchObject({ id: 'job-one', status: 'pending' });
    expect(xhr.method).toBe('POST');
    expect(relativeURL(xhr.url)).toBe('/api/v1/uploads');
    expect(xhr.withCredentials).toBe(true);
    expect(xhr.headers.get('X-Gooru-CSRF')).toBe('secret-token');
    expect(xhr.headers.get('Prefer')).toBe('respond-async');
    expect(xhr.body).toBeInstanceOf(FormData);
    expect((xhr.body as FormData).get('conflict_policy')).toBe('rename');
    expect((xhr.body as FormData).get('tags')).toBe('reviewed');
    expect(((xhr.body as FormData).get('files') as File).name).toBe('hello.txt');
    expect(progress.mock.calls.map(([value]) => value)).toEqual([20, 70, 100, 100]);
  });

  it('maps upload API errors from XMLHttpRequest responses', async () => {
    const unauthorized = vi.fn();
    setUnauthorizedHandler(unauthorized);
    installUploadXHR({
      status: 401,
      response: { error: { code: 'unauthorized', message: 'session expired' } }
    });

    const file = new File(['hello'], 'hello.txt', { type: 'text/plain' });
    await expect(new ApiClient('expired-token').uploadFiles([file], [], true, '', 'rename', vi.fn())).rejects.toMatchObject({
      status: 401,
      code: 'unauthorized',
      message: 'session expired'
    });
    expect(unauthorized).toHaveBeenCalledOnce();
    setUnauthorizedHandler(undefined);
  });

  it('fetches jobs with cookies and cancels with CSRF', async () => {
    const requests: Array<{ url: string; method?: string; headers: Headers }> = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      requests.push({
        url: relativeURL(request.url),
        method: request.method === 'GET' ? undefined : request.method,
        headers: request.headers
      });
      return Response.json({ id: 'job-one', type: 'upload_import', status: 'canceled' });
    }) as typeof fetch;

    const client = new ApiClient('secret-token');
    await client.getJob('job-one');
    await client.cancelJob('job-one');

    expect(requests).toHaveLength(2);
    expect(requests[0].url).toBe('/api/v1/jobs/job-one');
    expect(requests[0].method).toBeUndefined();
    expect(requests[0].headers.get('Authorization')).toBeNull();
    expect(requests[0].headers.get('X-Gooru-CSRF')).toBeNull();
    expect(requests[1].url).toBe('/api/v1/jobs/job-one');
    expect(requests[1].method).toBe('DELETE');
    expect(requests[1].headers.get('Authorization')).toBeNull();
    expect(requests[1].headers.get('X-Gooru-CSRF')).toBe('secret-token');
  });
});

interface FakeXHRConfig {
  status: number;
  response: unknown;
  progress?: Array<[number, number]>;
}

class FakeEventTarget {
  private listeners = new Map<string, Array<(event: ProgressEvent<EventTarget>) => void>>();

  addEventListener(type: string, listener: EventListenerOrEventListenerObject | null) {
    if (!listener) return;
    const callback = typeof listener === 'function' ? listener : (event: Event) => listener.handleEvent(event);
    const callbacks = this.listeners.get(type) ?? [];
    callbacks.push(callback as (event: ProgressEvent<EventTarget>) => void);
    this.listeners.set(type, callbacks);
  }

  emit(type: string, event: Partial<ProgressEvent<EventTarget>> = {}) {
    for (const listener of this.listeners.get(type) ?? []) {
      listener(event as ProgressEvent<EventTarget>);
    }
  }
}

class FakeXHR extends FakeEventTarget {
  method = '';
  url = '';
  status = 0;
  responseText = '';
  withCredentials = false;
  body: Document | XMLHttpRequestBodyInit | null = null;
  headers = new Headers();
  upload = new FakeEventTarget() as unknown as XMLHttpRequestUpload;
  private readonly config: FakeXHRConfig;

  constructor(config: FakeXHRConfig) {
    super();
    this.config = config;
  }

  open(method: string, url: string | URL) {
    this.method = method;
    this.url = String(url);
  }

  setRequestHeader(name: string, value: string) {
    this.headers.set(name, value);
  }

  send(body?: Document | XMLHttpRequestBodyInit | null) {
    this.body = body ?? null;
    for (const [loaded, total] of this.config.progress ?? []) {
      (this.upload as unknown as FakeEventTarget).emit('progress', { lengthComputable: true, loaded, total });
    }
    this.status = this.config.status;
    this.responseText = JSON.stringify(this.config.response);
    this.emit('load');
  }
}

function installUploadXHR(config: FakeXHRConfig): FakeXHR {
  const xhr = new FakeXHR(config);
  globalThis.XMLHttpRequest = class {
    constructor() {
      return xhr as unknown as XMLHttpRequest;
    }
  } as unknown as typeof XMLHttpRequest;
  return xhr;
}

async function bodyText(body: BodyInit | null | undefined) {
  if (!body) return '';
  if (typeof body === 'string') return body;
  return new Response(body).text();
}

function relativeURL(url: string) {
  const parsed = new URL(url, 'http://localhost');
  return `${parsed.pathname}${parsed.search}`;
}
