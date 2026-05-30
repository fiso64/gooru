import { describe, expect, it, vi } from 'vitest';
import { AuthenticatedMediaCache } from './authenticated';

describe('AuthenticatedMediaCache', () => {
  it('shares thumbnail fetches and revokes object URLs after all leases release', async () => {
    const fetcher = vi.fn(async () => new Response(new Blob(['image']), { status: 200 })) as unknown as typeof fetch;
    const objectURLs = {
      createObjectURL: vi.fn(() => 'blob:thumbnail'),
      revokeObjectURL: vi.fn()
    };
    const cache = new AuthenticatedMediaCache(fetcher, objectURLs);

    const [first, second] = await Promise.all([
      cache.load('/api/v1/files/id/thumbnail?size=256', 'token'),
      cache.load('/api/v1/files/id/thumbnail?size=256', 'token')
    ]);

    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(first.url).toBe('blob:thumbnail');
    expect(second.url).toBe('blob:thumbnail');
    first.release();
    expect(objectURLs.revokeObjectURL).not.toHaveBeenCalled();
    second.release();
    expect(objectURLs.revokeObjectURL).toHaveBeenCalledWith('blob:thumbnail');
  });

  it('does not cache failed media responses', async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(new Response('', { status: 415 }))
      .mockResolvedValueOnce(new Response(new Blob(['image']), { status: 200 })) as unknown as typeof fetch;
    const objectURLs = {
      createObjectURL: vi.fn(() => 'blob:thumbnail'),
      revokeObjectURL: vi.fn()
    };
    const cache = new AuthenticatedMediaCache(fetcher, objectURLs);

    await expect(cache.load('/thumbnail', 'token')).rejects.toThrow('media request failed: 415');
    const lease = await cache.load('/thumbnail', 'token');

    expect(fetcher).toHaveBeenCalledTimes(2);
    expect(lease.url).toBe('blob:thumbnail');
    lease.release();
  });

  it('revokes a late object URL when a request is aborted before blob decoding finishes', async () => {
    let resolveBlob: (blob: Blob) => void = () => {};
    const blob = new Promise<Blob>((resolve) => {
      resolveBlob = resolve;
    });
    const fetcher = vi.fn(async () => ({ ok: true, blob: () => blob }) as Response) as unknown as typeof fetch;
    const objectURLs = {
      createObjectURL: vi.fn(() => 'blob:late-thumbnail'),
      revokeObjectURL: vi.fn()
    };
    const cache = new AuthenticatedMediaCache(fetcher, objectURLs);
    const controller = new AbortController();

    const loading = cache.load('/thumbnail', 'token', controller.signal);
    controller.abort();
    resolveBlob(new Blob(['image']));

    await expect(loading).rejects.toMatchObject({ name: 'AbortError' });
    expect(objectURLs.createObjectURL).toHaveBeenCalledTimes(1);
    expect(objectURLs.revokeObjectURL).toHaveBeenCalledWith('blob:late-thumbnail');
  });
});
