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
});
