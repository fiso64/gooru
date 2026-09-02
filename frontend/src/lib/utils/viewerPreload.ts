import { viewerImageSource, type ViewerMedia } from './media';

export type PreloadableViewerMedia = ViewerMedia & { id: string };

type PreloadEntry = {
  promise: Promise<void>;
  dispose: () => void;
};

const preloadCache = new Map<string, PreloadEntry>();
const maxCachedPreloads = 6;

export function viewerPreloadSource(file: PreloadableViewerMedia): string {
  if (file.media_kind === 'video' || file.media_kind === 'audio' || file.media_type.startsWith('audio/')) {
    return file.media_urls.content;
  }
  return viewerImageSource(file, false);
}

function remember(key: string, entry: PreloadEntry): Promise<void> {
  preloadCache.set(key, entry);
  while (preloadCache.size > maxCachedPreloads) {
    const oldest = preloadCache.keys().next().value as string | undefined;
    if (oldest == null) break;
    const evicted = preloadCache.get(oldest);
    preloadCache.delete(oldest);
    evicted?.dispose();
  }
  return entry.promise;
}

function preloadImage(source: string): PreloadEntry {
  const image = new Image();
  const promise = new Promise<void>((resolve, reject) => {
    image.onload = () => {
      const decoded = typeof image.decode === 'function' ? image.decode() : Promise.resolve();
      decoded.then(resolve, resolve);
    };
    image.onerror = () => reject(new Error(`Unable to preload image ${source}`));
    image.src = source;
  });
  return {
    promise,
    dispose: () => {
      image.onload = null;
      image.onerror = null;
      image.src = '';
    }
  };
}

function preloadMedia(source: string, kind: 'video' | 'audio'): PreloadEntry {
  const media = document.createElement(kind);
  const promise = new Promise<void>((resolve, reject) => {
    const loaded = () => {
      media.removeEventListener('loadeddata', loaded);
      media.removeEventListener('error', failed);
      resolve();
    };
    const failed = () => {
      media.removeEventListener('loadeddata', loaded);
      media.removeEventListener('error', failed);
      reject(new Error(`Unable to preload ${kind} ${source}`));
    };
    media.preload = 'auto';
    media.addEventListener('loadeddata', loaded, { once: true });
    media.addEventListener('error', failed, { once: true });
    media.src = source;
    media.load();
  });
  return {
    promise,
    dispose: () => {
      media.removeAttribute('src');
      media.load();
    }
  };
}

export function preloadViewerMediaSource(file: PreloadableViewerMedia, source: string): Promise<void> {
  if (typeof window === 'undefined' || !source) return Promise.resolve();
  const key = `${file.id}|${source}`;
  const cached = preloadCache.get(key);
  if (cached) return cached.promise;

  const kind = file.media_kind === 'video'
    ? 'video'
    : (file.media_kind === 'audio' || file.media_type.startsWith('audio/') ? 'audio' : 'image');
  const entry = kind === 'image' ? preloadImage(source) : preloadMedia(source, kind);
  entry.promise = entry.promise.catch((error) => {
    const current = preloadCache.get(key);
    if (current === entry) preloadCache.delete(key);
    entry.dispose();
    throw error;
  });
  return remember(key, entry);
}

export function preloadViewerMedia(file: PreloadableViewerMedia): Promise<void> {
  return preloadViewerMediaSource(file, viewerPreloadSource(file));
}

export function clearViewerPreloadCache(): void {
  for (const entry of preloadCache.values()) entry.dispose();
  preloadCache.clear();
}
