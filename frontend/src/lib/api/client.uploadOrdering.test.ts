import { afterEach, describe, expect, it } from 'vitest';
import { ApiClient } from './client';

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

class UploadXHR extends FakeEventTarget {
  status = 202;
  responseText = JSON.stringify({ id: 'operation-test', kind: 'upload_import', status: 'pending' });
  withCredentials = false;
  upload = new FakeEventTarget() as unknown as XMLHttpRequestUpload;
  body: XMLHttpRequestBodyInit | null = null;
  headers = new Headers();

  open() {}
  setRequestHeader(name: string, value: string) { this.headers.set(name, value); }
  abort() { this.emit('abort'); }
  send(body?: Document | XMLHttpRequestBodyInit | null) {
    this.body = body as XMLHttpRequestBodyInit | null;
    this.emit('load');
    this.emit('loadend');
  }
}

let xhr: UploadXHR;
const originalXHR = globalThis.XMLHttpRequest;
const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.XMLHttpRequest = originalXHR;
  globalThis.fetch = originalFetch;
});

function installXHR() {
  xhr = new UploadXHR();
  globalThis.XMLHttpRequest = class {
    constructor() { return xhr as unknown as XMLHttpRequest; }
  } as unknown as typeof XMLHttpRequest;
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = new Request(input, init);
    expect(new URL(request.url).pathname).toBe('/api/v1/uploads');
    expect(request.headers.get('X-Gooru-Upload-Reserve')).toBe('true');
    return Response.json({ id: 'operation-reserved', kind: 'upload_import', status: 'pending', progress_total: 1, progress_completed: 0, progress_failed: 0 }, { status: 201 });
  }) as typeof fetch;
}

function files(count: number) {
  return Array.from({ length: count }, (_, index) => new File([String(index)], `file-${index}.jpg`, { lastModified: 1_700_000_000_000 + index }));
}

describe('upload ordering multipart metadata', () => {
  it('omits canonical positional ordering fields that the server derives by default', async () => {
    installXHR();
    const batch = files(4);

    await new ApiClient().uploadFiles(batch, [], true, '', 'rename', undefined, {
      queueIndex: [0, 1, 2, 3],
      queueTotal: [4, 4, 4, 4]
    });

    const form = xhr.body as FormData;
    expect(xhr.headers.get('X-Gooru-Upload-Operation-ID')).toBe('operation-reserved');
    expect(form.getAll('queue_index')).toEqual([]);
    expect(form.getAll('queue_total')).toEqual([]);
    expect(form.getAll('files')).toHaveLength(4);
  });

  it('serializes aligned per-file tags including explicit empty sets', async () => {
    installXHR();
    const batch = files(2);

    await new ApiClient().uploadFiles(batch, ['fallback'], true, '', 'rename', undefined, {
      itemTags: [['item:one'], []]
    });

    const form = xhr.body as FormData;
    expect(form.getAll('item_tags')).toEqual(['item:one', '']);
  });

  it('preserves explicit non-canonical ordering metadata', async () => {
    installXHR();
    const batch = files(2);

    await new ApiClient().uploadFiles(batch, [], true, '', 'rename', undefined, {
      queueIndex: [2, 3],
      queueTotal: [4, 4]
    });

    const form = xhr.body as FormData;
    expect(form.getAll('queue_index')).toEqual(['2', '3']);
    expect(form.getAll('queue_total')).toEqual(['4', '4']);
  });
});
