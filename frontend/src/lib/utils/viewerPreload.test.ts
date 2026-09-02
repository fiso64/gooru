import { describe, expect, it } from 'vitest';
import { viewerPreloadSource, type PreloadableViewerMedia } from './viewerPreload';

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
});
