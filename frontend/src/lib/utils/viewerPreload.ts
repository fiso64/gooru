import { isPDFViewerMedia, viewerImageSource, type ViewerMedia } from './media';

export type PreloadableViewerMedia = ViewerMedia & { id: string };

export type ViewerPreloadResult = {
  width?: number;
  height?: number;
};

type PreloadEntry = {
  promise: Promise<ViewerPreloadResult>;
  dispose: () => void;
};

type SlotRelease = () => void;

const preloadCache = new Map<string, PreloadEntry>();
const maxCachedPreloads = 6;
const maxConcurrentImageDecodes = 2;
let activeImageDecodes = 0;
const imageDecodeWaiters: Array<(release: SlotRelease) => void> = [];

export function viewerPreloadSource(file: PreloadableViewerMedia): string {
  if (file.media_kind === 'video' || file.media_kind === 'audio' || file.media_type.startsWith('audio/')) {
    return file.media_urls.content;
  }
  return viewerImageSource(file, false);
}

function releaseImageDecodeSlot(): void {
  const next = imageDecodeWaiters.shift();
  if (next) {
    next(releaseImageDecodeSlot);
    return;
  }
  activeImageDecodes = Math.max(0, activeImageDecodes - 1);
}

function acquireImageDecodeSlot(): Promise<SlotRelease> {
  if (activeImageDecodes < maxConcurrentImageDecodes) {
    activeImageDecodes += 1;
    return Promise.resolve(releaseImageDecodeSlot);
  }
  return new Promise((resolve) => imageDecodeWaiters.push(resolve));
}

function remember(key: string, entry: PreloadEntry): Promise<ViewerPreloadResult> {
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
  let image: HTMLImageElement | undefined;
  let cancelled = false;
  let settled = false;
  let releaseSlot: SlotRelease | undefined;
  let rejectPromise: ((reason?: unknown) => void) | undefined;

  const finish = (resolve?: (result: ViewerPreloadResult) => void, reject?: (reason?: unknown) => void, result?: ViewerPreloadResult, error?: unknown) => {
    if (settled) return;
    settled = true;
    if (image) {
      image.onload = null;
      image.onerror = null;
    }
    const release = releaseSlot;
    releaseSlot = undefined;
    release?.();
    if (error) reject?.(error);
    else resolve?.(result ?? {});
  };

  const promise = new Promise<ViewerPreloadResult>((resolve, reject) => {
    rejectPromise = reject;
    void acquireImageDecodeSlot().then((release) => {
      releaseSlot = release;
      if (cancelled) {
        finish(undefined, reject, undefined, new Error(`Cancelled image preload ${source}`));
        return;
      }

      image = new Image();
      image.onload = () => {
        const decoded = typeof image!.decode === 'function' ? image!.decode() : Promise.resolve();
        decoded.then(
          () => finish(resolve, reject, { width: image!.naturalWidth, height: image!.naturalHeight }),
          () => finish(resolve, reject, { width: image!.naturalWidth, height: image!.naturalHeight })
        );
      };
      image.onerror = () => finish(undefined, reject, undefined, new Error(`Unable to preload image ${source}`));
      image.src = source;
    });
  });

  return {
    promise,
    dispose: () => {
      cancelled = true;
      if (!image || settled) return;
      image.onload = null;
      image.onerror = null;
      image.src = '';
      finish(undefined, rejectPromise, undefined, new Error(`Cancelled image preload ${source}`));
    }
  };
}

function preloadMedia(source: string, kind: 'video' | 'audio'): PreloadEntry {
  const media = document.createElement(kind);
  const promise = new Promise<ViewerPreloadResult>((resolve, reject) => {
    const loaded = () => {
      media.removeEventListener('loadeddata', loaded);
      media.removeEventListener('error', failed);
      resolve({});
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

export function preloadViewerMediaSource(file: PreloadableViewerMedia, source: string): Promise<ViewerPreloadResult> {
  // The browser's native PDF viewer streams the original on demand. Never
  // speculatively fetch a potentially large document through Image preloading.
  if (typeof window === 'undefined' || !source || isPDFViewerMedia(file)) return Promise.resolve({});
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

export function preloadViewerMedia(file: PreloadableViewerMedia): Promise<ViewerPreloadResult> {
  return preloadViewerMediaSource(file, viewerPreloadSource(file));
}

export function clearViewerPreloadCache(): void {
  for (const entry of preloadCache.values()) entry.dispose();
  preloadCache.clear();
}
