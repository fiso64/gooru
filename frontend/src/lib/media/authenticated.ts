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
}

export class AuthenticatedMediaCache {
  private readonly entries = new Map<string, CacheEntry>();

  constructor(
    private readonly fetcher: typeof fetch = fetch,
    private readonly objectURLs: ObjectURLStore = URL
  ) {}

  async load(url: string, token: string): Promise<MediaLease> {
    const key = `${token}\n${url}`;
    let entry = this.entries.get(key);
    if (!entry) {
      entry = {
        refs: 0,
        objectURL: '',
        promise: this.fetcher(url, {
          headers: { Authorization: `Bearer ${token}` }
        })
          .then(async (response) => {
            if (!response.ok) throw new Error(`media request failed: ${response.status}`);
            return response.blob();
          })
          .then((blob) => {
            const objectURL = this.objectURLs.createObjectURL(blob);
            const current = this.entries.get(key);
            if (current) current.objectURL = objectURL;
            return objectURL;
          })
      };
      this.entries.set(key, entry);
    }

    entry.refs += 1;
    try {
      const objectURL = entry.objectURL || (await entry.promise);
      return {
        url: objectURL,
        release: () => this.release(key)
      };
    } catch (error) {
      this.release(key);
      throw error;
    }
  }

  private release(key: string) {
    const entry = this.entries.get(key);
    if (!entry) return;
    entry.refs -= 1;
    if (entry.refs > 0) return;
    if (entry.objectURL) this.objectURLs.revokeObjectURL(entry.objectURL);
    this.entries.delete(key);
  }
}

export const authenticatedMediaCache = new AuthenticatedMediaCache();
