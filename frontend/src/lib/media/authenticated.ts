export interface MediaLease {
  url: string;
  release: () => void;
}

interface ObjectURLStore {
  createObjectURL(blob: Blob): string;
  revokeObjectURL(url: string): void;
}

interface CacheEntry {
  refs: number;
  objectURL: string;
  promise: Promise<string>;
  controller: AbortController;
}

export class AuthenticatedMediaCache {
  private readonly entries = new Map<string, CacheEntry>();

  constructor(
    private readonly fetcher: typeof fetch = fetch,
    private readonly objectURLs: ObjectURLStore = URL
  ) {}

  async load(url: string, signal?: AbortSignal): Promise<MediaLease> {
    if (signal?.aborted) throw abortError();

    const key = url;
    let entry = this.entries.get(key);
    if (!entry) {
      const controller = new AbortController();
      entry = {
        refs: 0,
        objectURL: '',
        controller,
        promise: this.fetcher(url, {
          credentials: 'same-origin',
          signal: controller.signal
        })
          .then(async (response) => {
            if (!response.ok) throw new Error(`media request failed: ${response.status}`);
            return response.blob();
          })
          .then((blob) => {
            const objectURL = this.objectURLs.createObjectURL(blob);
            const current = this.entries.get(key);
            if (current && current.refs > 0) {
              current.objectURL = objectURL;
            } else {
              this.objectURLs.revokeObjectURL(objectURL);
              throw abortError();
            }
            return objectURL;
          })
      };
      this.entries.set(key, entry);
    }

    entry.refs += 1;
    let releasedBySignal = false;
    const releaseForSignal = () => {
      releasedBySignal = true;
      this.release(key);
    };
    signal?.addEventListener('abort', releaseForSignal, { once: true });

    try {
      const objectURL = entry.objectURL || (await entry.promise);
      if (signal?.aborted) throw abortError();
      let released = false;
      return {
        url: objectURL,
        release: () => {
          if (released) return;
          released = true;
          this.release(key);
        }
      };
    } catch (error) {
      if (!releasedBySignal) this.release(key);
      throw error;
    } finally {
      signal?.removeEventListener('abort', releaseForSignal);
    }
  }

  private release(key: string) {
    const entry = this.entries.get(key);
    if (!entry) return;
    if (entry.refs <= 0) return;
    entry.refs -= 1;
    if (entry.refs > 0) return;
    entry.controller.abort();
    if (entry.objectURL) this.objectURLs.revokeObjectURL(entry.objectURL);
    this.entries.delete(key);
  }
}

export const authenticatedMediaCache = new AuthenticatedMediaCache();

function abortError() {
  return new DOMException('media request aborted', 'AbortError');
}
