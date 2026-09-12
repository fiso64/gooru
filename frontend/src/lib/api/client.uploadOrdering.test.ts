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

  open() {}
  setRequestHeader() {}
  abort() { this.emit('abort'); }
  send(body?: Document | XMLHttpRequestBodyInit | null) {
    this.body = body as XMLHttpRequestBodyInit | null;
    this.emit('load');
    this.emit('loadend');
  }
}

let xhr: UploadXHR;
const originalXHR = globalThis.XMLHttpRequest;

afterEach(() => {
  globalThis.XMLHttpRequest = originalXHR;
});

function installXHR() {
  xhr = new UploadXHR();
  globalThis.XMLHttpRequest = class {
    constructor() { return xhr as unknown as XMLHttpRequest; }
  } as unknown as typeof XMLHttpRequest;
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
    expect(form.getAll('queue_index')).toEqual([]);
    expect(form.getAll('queue_total')).toEqual([]);
    expect(form.getAll('files')).toHaveLength(4);
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
