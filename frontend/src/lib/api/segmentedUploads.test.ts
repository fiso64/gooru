import { afterEach, describe, expect, it } from 'vitest';
import { uploadSegmentedFiles } from './segmentedUploads';

const originalXMLHttpRequest = globalThis.XMLHttpRequest;
const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.XMLHttpRequest = originalXMLHttpRequest;
  globalThis.fetch = originalFetch;
});

describe('uploadSegmentedFiles', () => {
  it('reserves once with the logical segment count and sends stable segment identity', async () => {
    const requests: Request[] = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      requests.push(request);
      return Response.json({
        id: 'operation-reserved',
        kind: 'upload_import',
        status: 'pending',
        progress_total: 3,
        progress_completed: 0,
        progress_failed: 0
      }, { status: 201 });
    }) as typeof fetch;
    const xhr = installUploadXHR({ id: 'operation-reserved', kind: 'upload_import', status: 'pending' });
    const file = new File(['hello'], 'hello.txt', { type: 'text/plain', lastModified: 1_700_000_000_000 });

    const result = await uploadSegmentedFiles('secret-token', {
      files: [file],
      tags: ['reviewed'],
      targetID: 'default',
      conflictPolicy: 'rename',
      addedAtStrategy: 'queue',
      queueTimeMs: [1_700_000_000_000],
      queueFirstTimeMs: 1_700_000_000_000,
      queueLastTimeMs: 1_700_000_000_000,
      queueIndex: [1000],
      queueTotal: [2001],
      segmentIndex: 0,
      segmentCount: 3
    });

    expect(result.id).toBe('operation-reserved');
    expect(requests).toHaveLength(1);
    expect(relativeURL(requests[0].url)).toBe('/api/v1/uploads');
    expect(requests[0].method).toBe('POST');
    expect(requests[0].headers.get('X-Gooru-CSRF')).toBe('secret-token');
    expect(requests[0].headers.get('X-Gooru-Upload-Reserve')).toBe('true');
    expect(requests[0].headers.get('X-Gooru-Upload-Segment-Count')).toBe('3');
    expect(xhr.headers.get('Prefer')).toBe('respond-async');
    expect(xhr.headers.get('X-Gooru-Upload-Operation-ID')).toBe('operation-reserved');
    expect(xhr.headers.get('X-Gooru-Upload-Segment-Index')).toBe('0');
    expect((xhr.body as FormData).get('queue_index')).toBe('1000');
    expect((xhr.body as FormData).get('queue_total')).toBe('2001');
  });

  it('serializes aligned per-file tags including explicit empty sets', async () => {
    globalThis.fetch = (async () => {
      throw new Error('unexpected reservation request');
    }) as typeof fetch;
    const xhr = installUploadXHR({ id: 'operation-existing', kind: 'upload_import', status: 'pending' });
    const files = [
      new File(['one'], 'one.txt', { type: 'text/plain' }),
      new File(['two'], 'two.txt', { type: 'text/plain' })
    ];

    await uploadSegmentedFiles('secret-token', {
      files,
      tags: ['fallback'],
      itemTags: [['item:one'], []],
      targetID: 'default',
      conflictPolicy: 'rename',
      operationID: 'operation-existing',
      segmentIndex: 1,
      segmentCount: 2
    });

    expect((xhr.body as FormData).getAll('item_tags')).toEqual(['item:one', '']);
  });

  it('reuses an existing logical operation without reserving again', async () => {
    globalThis.fetch = (async () => {
      throw new Error('unexpected reservation request');
    }) as typeof fetch;
    const xhr = installUploadXHR({ id: 'operation-existing', kind: 'upload_import', status: 'pending' });
    const file = new File(['later'], 'later.txt', { type: 'text/plain' });

    const result = await uploadSegmentedFiles('secret-token', {
      files: [file],
      tags: [],
      targetID: 'default',
      conflictPolicy: 'rename',
      operationID: 'operation-existing',
      segmentIndex: 2,
      segmentCount: 3
    });

    expect(result.id).toBe('operation-existing');
    expect(xhr.headers.get('X-Gooru-Upload-Operation-ID')).toBe('operation-existing');
    expect(xhr.headers.get('X-Gooru-Upload-Segment-Index')).toBe('2');
  });
});

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
    for (const listener of this.listeners.get(type) ?? []) listener(event as ProgressEvent<EventTarget>);
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
  private readonly response: unknown;

  constructor(response: unknown) {
    super();
    this.response = response;
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
    this.status = 202;
    this.responseText = JSON.stringify(this.response);
    this.emit('load');
    this.emit('loadend');
  }

  abort() {
    this.emit('abort');
    this.emit('loadend');
  }
}

function installUploadXHR(response: unknown): FakeXHR {
  const xhr = new FakeXHR(response);
  globalThis.XMLHttpRequest = class {
    constructor() {
      return xhr as unknown as XMLHttpRequest;
    }
  } as unknown as typeof XMLHttpRequest;
  return xhr;
}

function relativeURL(url: string) {
  const parsed = new URL(url, 'http://localhost');
  return `${parsed.pathname}${parsed.search}`;
}
