const STORAGE_PREFIX = 'gooru.preference.v1.';

type ReadWriteStorage = Pick<Storage, 'getItem' | 'setItem'>;
type MutableStorage = ReadWriteStorage & Pick<Storage, 'removeItem'>;

export const browserPersistenceRegistry = {
  commonTagsCollapsed: { scope: 'local', key: 'common-tags.collapsed' },
  libraryPaginationMode: { scope: 'local', key: 'library.pagination-mode' },
  viewerSessionPreferences: { scope: 'session', key: 'gooru.viewer.preferences.v1' }
} as const;

type PersistenceRegistration = (typeof browserPersistenceRegistry)[keyof typeof browserPersistenceRegistry];
type PersistenceKeyForScope<Scope extends PersistenceRegistration['scope']> = Extract<
  PersistenceRegistration,
  { scope: Scope }
>['key'];

export type BrowserLocalPreferenceKey = PersistenceKeyForScope<'local'>;
export type BrowserSessionPreferenceKey = PersistenceKeyForScope<'session'>;

function browserLocalStorage(): MutableStorage | undefined {
  if (typeof window === 'undefined') return undefined;
  try {
    return window.localStorage;
  } catch {
    return undefined;
  }
}

function browserSessionStorage(): ReadWriteStorage | undefined {
  if (typeof window === 'undefined') return undefined;
  try {
    return window.sessionStorage;
  } catch {
    return undefined;
  }
}

function localStorageKey(key: BrowserLocalPreferenceKey) {
  return `${STORAGE_PREFIX}${key}`;
}

export function readBrowserPreference<T>(
  key: BrowserLocalPreferenceKey,
  fallback: T,
  isValid: (value: unknown) => value is T,
  storage: ReadWriteStorage | undefined = browserLocalStorage()
): T {
  return readStoredJSON(localStorageKey(key), fallback, isValid, storage);
}

export function writeBrowserPreference<T>(
  key: BrowserLocalPreferenceKey,
  value: T,
  storage: ReadWriteStorage | undefined = browserLocalStorage()
): boolean {
  return writeStoredJSON(localStorageKey(key), value, storage);
}

export function clearBrowserPreference(
  key: BrowserLocalPreferenceKey,
  storage: MutableStorage | undefined = browserLocalStorage()
): boolean {
  return clearStoredValue(localStorageKey(key), storage);
}

export function readBrowserSessionPreference<T>(
  key: BrowserSessionPreferenceKey,
  fallback: T,
  isValid: (value: unknown) => value is T,
  storage: ReadWriteStorage | undefined = browserSessionStorage()
): T {
  return readStoredJSON(key, fallback, isValid, storage);
}

export function writeBrowserSessionPreference<T>(
  key: BrowserSessionPreferenceKey,
  value: T,
  storage: ReadWriteStorage | undefined = browserSessionStorage()
): boolean {
  return writeStoredJSON(key, value, storage);
}

function readStoredJSON<T>(
  key: string,
  fallback: T,
  isValid: (value: unknown) => value is T,
  storage: ReadWriteStorage | undefined
): T {
  if (!storage) return fallback;
  try {
    const raw = storage.getItem(key);
    if (raw === null) return fallback;
    const value: unknown = JSON.parse(raw);
    return isValid(value) ? value : fallback;
  } catch {
    return fallback;
  }
}

function writeStoredJSON<T>(key: string, value: T, storage: ReadWriteStorage | undefined): boolean {
  if (!storage) return false;
  try {
    storage.setItem(key, JSON.stringify(value));
    return true;
  } catch {
    return false;
  }
}

function clearStoredValue(key: string, storage: MutableStorage | undefined): boolean {
  if (!storage) return false;
  try {
    storage.removeItem(key);
    return true;
  } catch {
    return false;
  }
}
