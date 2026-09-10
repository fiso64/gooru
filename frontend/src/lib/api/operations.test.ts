import { afterEach, describe, expect, it, vi } from 'vitest';
import { cancelBackgroundOperation, listBackgroundOperationsByIDs } from './operations';

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

describe('durable operation transport', () => {
  it('batches requested operation IDs with cookie authentication', async () => {
    let request: Request | undefined;
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      request = new Request(input, init);
      return Response.json({ items: [] });
    }) as typeof fetch;

    await listBackgroundOperationsByIDs(['operation one', 'operation-two']);

    expect(request).toBeDefined();
    expect(request?.credentials).toBe('same-origin');
    expect(new URL(request!.url, 'http://localhost').searchParams.getAll('id')).toEqual(['operation one', 'operation-two']);
  });

  it('cancels with the server CSRF header', async () => {
    let request: Request | undefined;
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      request = new Request(input, init);
      return Response.json({
        id: 'operation/one',
        kind: 'upload.import',
        status: 'canceled',
        progress_total: 1,
        progress_completed: 0,
        progress_failed: 0,
        created_at: '2026-09-10T00:00:00Z'
      });
    }) as typeof fetch;

    await cancelBackgroundOperation('operation/one', 'secret-token');

    expect(request).toBeDefined();
    expect(request?.method).toBe('DELETE');
    expect(request?.credentials).toBe('same-origin');
    expect(request?.headers.get('X-Gooru-CSRF')).toBe('secret-token');
    expect(request?.headers.get('X-CSRF-Token')).toBeNull();
    expect(new URL(request!.url, 'http://localhost').pathname).toBe('/api/v1/operations/operation%2Fone');
  });
});
