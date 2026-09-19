import { describe, expect, it } from 'vitest';
import { canUseOriginalInViewer, preserveNativeViewerSize, viewerImageSource, type ViewerMedia } from './media';

function media(overrides: Partial<ViewerMedia> = {}): ViewerMedia {
  return {
    media_kind: 'photo',
    media_type: 'image/jpeg',
    media_urls: {
      content: '/content/original',
      preview: '/preview/derived'
    },
    ...overrides
  };
}

describe('viewer media source policy', () => {
  it('uses derived previews for normal images by default', () => {
    expect(viewerImageSource(media(), false)).toBe('/preview/derived');
  });

  it('uses original image content when requested for generated photo kinds', () => {
    expect(viewerImageSource(media(), true)).toBe('/content/original');
  });

  it('prefers an available lossless display derivative only in configured full-image mode', () => {
    const file = media({ media_urls: {
      content: '/content/original',
      preview: '/preview/derived',
      lossless: '/lossless/display'
    } });
    expect(viewerImageSource(file, true, true)).toBe('/lossless/display');
    expect(viewerImageSource(file, true, false)).toBe('/content/original');
    expect(viewerImageSource(file, false, true)).toBe('/preview/derived');
  });

  it('uses original content if the lossless derivative is unavailable', () => {
    expect(viewerImageSource(media(), true, true)).toBe('/content/original');
  });

  it('always uses original GIF content so animation is preserved without restricting fit scaling', () => {
    const gif = media({ media_kind: 'gif', media_type: 'image/gif' });
    expect(viewerImageSource(gif, false)).toBe('/content/original');
    expect(preserveNativeViewerSize(gif)).toBe(false);
  });

  it('falls back to the preview when original content is unavailable', () => {
    const file = media({ media_urls: { content: '', preview: '/preview/derived' } });
    expect(canUseOriginalInViewer(file)).toBe(false);
    expect(viewerImageSource(file, true)).toBe('/preview/derived');
  });

  it('does not opt non-image fallback renderers into original content', () => {
    const file = media({ media_kind: 'other', media_type: 'application/pdf' });
    expect(canUseOriginalInViewer(file)).toBe(false);
    expect(viewerImageSource(file, true)).toBe('/preview/derived');
  });
});
