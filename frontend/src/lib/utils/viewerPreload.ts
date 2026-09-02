import { viewerImageSource, type ViewerMedia } from './media';

export type PreloadableViewerMedia = ViewerMedia & { id: string };

const preloadCache = new Map<string, Promise<void>>();
const maxCachedPreloads = 6;

export function viewerPreloadSource(file: PreloadableViewerMedia): string {
  if (file.media_kind === 'video' || file.media_kind === 'audio' || file.media_type.startsWith('audio/')) {
    return file.media_urls.content;
  }
  return viewerImageSource(file, false);
}

function remember(key: string, promise: Promise<void>): Promise<void> {
  preloadCache.set(key, promise);
  while (preloadCache.size > maxCachedPreloads) {
    const oldest = preloadCache.keys().next().value as string | undefined;
    if (oldest == null) break;
    preloadCache.delete(oldest);
  }
  return promise;
}

function preloadImage(source: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => {
      const decoded = typeof image.decode === 'function' ? image.decode() : Promise.resolve();
      decoded.then(resolve, resolve);
    };
    image.onerror = () => reject(new Error(`Unable to preload image ${source}`));
    image.src = source;
  });
}

function preloadMedia(source: string, kind: 'video' | 'audio'): Promise<void> {
  return new Promise((resolve, reject) => {
    const media = document.createElement(kind);
    const cleanup = () => {
      media.removeEventListener('loadeddata', loaded);
      media.removeEventListener('error', failed);
      media.removeAttribute('src');
      media.load();
    };
    const loaded = () => {
      cleanup();
      resolve();
    };
    const failed = () => {
      cleanup();
      reject(new Error(`Unable to preload ${kind} ${source}`));
    };
    media.preload = 'auto';
    media.addEventListener('loadeddata', loaded, { once: true });
    media.addEventListener('error', failed, { once: true });
    media.src = source;
    media.load();
  });
}

export function preloadViewerMedia(file: PreloadableViewerMedia): Promise<void> {
  if (typeof window === 'undefined') return Promise.resolve();
  const source = viewerPreloadSource(file);
  if (!source) return Promise.resolve();
  const key = `${file.id}|${source}`;
  const cached = preloadCache.get(key);
  if (cached) return cached;

  const kind = file.media_kind === 'video'
    ? 'video'
    : (file.media_kind === 'audio' || file.media_type.startsWith('audio/') ? 'audio' : 'image');
  const promise = kind === 'image' ? preloadImage(source) : preloadMedia(source, kind);
  return remember(key, promise.catch((error) => {
    preloadCache.delete(key);
    throw error;
  }));
}

export function clearViewerPreloadCache(): void {
  preloadCache.clear();
}
