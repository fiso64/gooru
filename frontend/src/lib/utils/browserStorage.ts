const STORAGE_PREFIX = 'gooru.preference.v1.';

type StorageLike = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;

function browserLocalStorage(): StorageLike | undefined {
  if (typeof window === 'undefined') return undefined;
  try {
    return window.localStorage;
  } catch {
    return undefined;
  }
}

function storageKey(key: string) {
  return `${STORAGE_PREFIX}${key}`;
}

export function readBrowserPreference<T>(
  key: string,
  fallback: T,
  isValid: (value: unknown) => value is T,
  storage: StorageLike | undefined = browserLocalStorage()
): T {
  if (!storage) return fallback;
  try {
    const raw = storage.getItem(storageKey(key));
    if (raw === null) return fallback;
    const value: unknown = JSON.parse(raw);
    return isValid(value) ? value : fallback;
  } catch {
    return fallback;
  }
}

export function writeBrowserPreference<T>(
  key: string,
  value: T,
  storage: StorageLike | undefined = browserLocalStorage()
): boolean {
  if (!storage) return false;
  try {
    storage.setItem(storageKey(key), JSON.stringify(value));
    return true;
  } catch {
    return false;
  }
}

export function clearBrowserPreference(
  key: string,
  storage: StorageLike | undefined = browserLocalStorage()
): boolean {
  if (!storage) return false;
  try {
    storage.removeItem(storageKey(key));
    return true;
  } catch {
    return false;
  }
}
