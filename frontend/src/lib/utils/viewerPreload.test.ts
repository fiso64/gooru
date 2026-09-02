import { afterEach, describe, expect, it, vi } from 'vitest';
import { clearViewerPreloadCache, preloadViewerMediaSource, viewerPreloadSource, type PreloadableViewerMedia } from './viewerPreload';

function media(overrides: Partial<PreloadableViewerMedia> = {}): PreloadableViewerMedia {
  return {
    id: 'file-1',
    media_kind: 'image',
    media_type: 'image/jpeg',
    media_urls: { content: '/content/file-1', preview: '/preview/file-1' },
    ...overrides
  };
}

describe('viewer preload source policy', () => {
  afterEach(() => {
    clearViewerPreloadCache();
    vi.unstubAllGlobals();
  });

  it('preloads the same derived source normal images show by default', () => {
    expect(viewerPreloadSource(media())).toBe('/preview/file-1');
  });

  it('preloads original GIF bytes so animation is decoded before navigation', () => {
    expect(viewerPreloadSource(media({ media_type: 'image/gif' }))).toBe('/content/file-1');
  });

  it('preloads playable content for video and audio', () => {
    expect(viewerPreloadSource(media({ media_kind: 'video', media_type: 'video/mp4' }))).toBe('/content/file-1');
    expect(viewerPreloadSource(media({ media_kind: 'audio', media_type: 'audio/mpeg' }))).toBe('/content/file-1');
  });

  it('keeps at most two image decodes in flight and continues queued work as slots free', async () => {
    const started: string[] = [];
    const pending = new Map<string, () => void>();

    class FakeImage {
      onload: (() => void) | null = null;
      onerror: (() => void) | null = null;
      naturalWidth = 100;
      naturalHeight = 80;
      private source = '';

      set src(value: string) {
        this.source = value;
        if (!value) return;
        queueMicrotask(() => this.onload?.());
      }

      get src() {
        return this.source;
      }

      decode() {
        started.push(this.source);
        return new Promise<void>((resolve) => pending.set(this.source, resolve));
      }
    }

    vi.stubGlobal('window', {});
    vi.stubGlobal('Image', FakeImage);

    const first = preloadViewerMediaSource(media({ id: '1' }), '/image/1');
    const second = preloadViewerMediaSource(media({ id: '2' }), '/image/2');
    const third = preloadViewerMediaSource(media({ id: '3' }), '/image/3');
    const fourth = preloadViewerMediaSource(media({ id: '4' }), '/image/4');

    await vi.waitFor(() => expect(started).toEqual(['/image/1', '/image/2']));

    pending.get('/image/1')?.();
    await first;
    await vi.waitFor(() => expect(started).toEqual(['/image/1', '/image/2', '/image/3']));
    expect(started).not.toContain('/image/4');

    pending.get('/image/2')?.();
    await second;
    await vi.waitFor(() => expect(started).toEqual(['/image/1', '/image/2', '/image/3', '/image/4']));

    pending.get('/image/3')?.();
    pending.get('/image/4')?.();
    await Promise.all([third, fourth]);
  });
});
