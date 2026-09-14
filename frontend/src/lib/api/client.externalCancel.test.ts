import { afterEach, describe, expect, it } from 'vitest';
import { ApiClient } from './client';

const originalXMLHttpRequest = globalThis.XMLHttpRequest;
const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.XMLHttpRequest = originalXMLHttpRequest;
  globalThis.fetch = originalFetch;
});

class FakeEventTarget {
  private listeners = new Map<string, Array<(event: Event) => void>>();

  addEventListener(type: string, listener: EventListenerOrEventListenerObject | null) {
    if (!listener) return;
    const callback = typeof listener === 'function' ? listener : (event: Event) => listener.handleEvent(event);
    const callbacks = this.listeners.get(type) ?? [];
    callbacks.push(callback);
    this.listeners.set(type, callbacks);
  }

  emit(type: string) {
    for (const listener of this.listeners.get(type) ?? []) listener(new Event(type));
  }
}

class ErrorXHR extends FakeEventTarget {
  withCredentials = false;
  status = 0;
  responseText = '';
  upload = new FakeEventTarget() as unknown as XMLHttpRequestUpload;

  open() {}
  setRequestHeader() {}
  send() { this.emit('error'); }
  abort() { this.emit('abort'); }
}

function installErrorXHR() {
  globalThis.XMLHttpRequest = class {
    constructor() { return new ErrorXHR() as unknown as XMLHttpRequest; }
  } as unknown as typeof XMLHttpRequest;
}

function operation(status: 'pending' | 'canceled') {
  return {
    id: 'operation-reserved',
    kind: 'upload_import',
    status,
    progress_total: 1,
    progress_completed: 0,
    progress_failed: 0,
    created_at: '2026-09-12T00:00:00Z'
  };
}

describe('upload transport errors after durable cancellation', () => {
  it('maps an externally canceled reservation to request_aborted without re-canceling it', async () => {
    installErrorXHR();
    const requests: Request[] = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      requests.push(request);
      const url = new URL(request.url);
      if (request.method === 'POST' && url.pathname === '/api/v1/uploads') {
        return Response.json(operation('pending'), { status: 201 });
      }
      if (request.method === 'GET' && url.pathname === '/api/v1/operations') {
        expect(url.searchParams.get('id')).toBe('operation-reserved');
        return Response.json({ items: [operation('canceled')] });
      }
      throw new Error(`unexpected request ${request.method} ${url.pathname}`);
    }) as typeof fetch;

    const file = new File(['hello'], 'hello.txt');
    await expect(new ApiClient('csrf').uploadFiles([file], [], true)).rejects.toMatchObject({
      code: 'request_aborted',
      message: 'Upload was canceled'
    });

    expect(requests.map((request) => request.method)).toEqual(['POST', 'GET']);
  });

  it('preserves a genuine network error and performs best-effort reservation cleanup', async () => {
    installErrorXHR();
    const methods: string[] = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      const url = new URL(request.url);
      methods.push(`${request.method} ${url.pathname}`);
      if (request.method === 'POST' && url.pathname === '/api/v1/uploads') {
        return Response.json(operation('pending'), { status: 201 });
      }
      if (request.method === 'GET' && url.pathname === '/api/v1/operations') {
        return Response.json({ items: [operation('pending')] });
      }
      if (request.method === 'DELETE' && url.pathname === '/api/v1/operations/operation-reserved') {
        return Response.json(operation('canceled'), { status: 202 });
      }
      throw new Error(`unexpected request ${request.method} ${url.pathname}`);
    }) as typeof fetch;

    const file = new File(['hello'], 'hello.txt');
    await expect(new ApiClient('csrf').uploadFiles([file], [], true)).rejects.toMatchObject({
      code: 'network_error',
      message: 'Network error while uploading files'
    });

    await Promise.resolve();
    expect(methods).toEqual([
      'POST /api/v1/uploads',
      'GET /api/v1/operations',
      'DELETE /api/v1/operations/operation-reserved'
    ]);
  });
});
