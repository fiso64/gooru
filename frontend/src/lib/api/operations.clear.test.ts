import { afterEach, describe, expect, it, vi } from 'vitest';
import { clearCompletedBackgroundOperations } from './operations';

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

describe('clear completed durable operations', () => {
  it('deletes the operation collection with the server CSRF header', async () => {
    let captured: { url: string; init?: RequestInit } | undefined;
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url;
      captured = { url, init };
      return Response.json({ cleared: 3 });
    }) as typeof fetch;

    const result = await clearCompletedBackgroundOperations('secret-token');

    expect(result).toEqual({ cleared: 3 });
    expect(captured?.init?.method).toBe('DELETE');
    expect(captured?.init?.credentials).toBe('same-origin');
    expect(new Headers(captured?.init?.headers).get('X-Gooru-CSRF')).toBe('secret-token');
    expect(new URL(captured!.url, 'http://localhost').pathname).toBe('/api/v1/operations');
  });
});
