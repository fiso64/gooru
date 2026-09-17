import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  cancelBackgroundOperation,
  listBackgroundOperationsByIDs,
  listMaintenanceJobs,
  runMaintenanceJob
} from './operations';

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

type CapturedFetch = { url: string; init?: RequestInit };

function captureURL(input: RequestInfo | URL): string {
  return typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url;
}

describe('durable operation transport', () => {
  it('batches requested operation IDs with cookie authentication', async () => {
    let request: CapturedFetch | undefined;
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      request = { url: captureURL(input), init };
      return Response.json({ items: [] });
    }) as typeof fetch;

    await listBackgroundOperationsByIDs(['operation one', 'operation-two']);

    expect(request).toBeDefined();
    expect(request?.init?.credentials).toBe('same-origin');
    expect(new URL(request!.url, 'http://localhost').searchParams.getAll('id')).toEqual(['operation one', 'operation-two']);
  });

  it('cancels with the server CSRF header', async () => {
    let request: CapturedFetch | undefined;
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      request = { url: captureURL(input), init };
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
    expect(request?.init?.method).toBe('DELETE');
    expect(request?.init?.credentials).toBe('same-origin');
    const headers = new Headers(request?.init?.headers);
    expect(headers.get('X-Gooru-CSRF')).toBe('secret-token');
    expect(headers.get('X-CSRF-Token')).toBeNull();
    expect(new URL(request!.url, 'http://localhost').pathname).toBe('/api/v1/operations/operation%2Fone');
  });

  it('discovers maintenance jobs from the backend catalog', async () => {
    let request: CapturedFetch | undefined;
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      request = { url: captureURL(input), init };
      return Response.json({
        items: [{ id: 'media-metadata-sweep', name: 'Extract media metadata', description: 'Scan pending files.' }]
      });
    }) as typeof fetch;

    const result = await listMaintenanceJobs();

    expect(result.items).toHaveLength(1);
    expect(result.items[0]?.id).toBe('media-metadata-sweep');
    expect(request?.init?.credentials).toBe('same-origin');
    expect(new URL(request!.url, 'http://localhost').pathname).toBe('/api/v1/maintenance-jobs');
  });

  it('invokes a selected maintenance job with CSRF protection', async () => {
    let request: CapturedFetch | undefined;
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      request = { url: captureURL(input), init };
      return Response.json({
        job: { id: 'job/with slash', name: 'Job', description: 'Description' },
        created: true
      }, { status: 202 });
    }) as typeof fetch;

    const result = await runMaintenanceJob('job/with slash', 'secret-token');

    expect(result.created).toBe(true);
    expect(request?.init?.method).toBe('POST');
    expect(request?.init?.credentials).toBe('same-origin');
    const headers = new Headers(request?.init?.headers);
    expect(headers.get('X-Gooru-CSRF')).toBe('secret-token');
    expect(new URL(request!.url, 'http://localhost').pathname).toBe('/api/v1/maintenance-jobs/job%2Fwith%20slash');
  });
});
