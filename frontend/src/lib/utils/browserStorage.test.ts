import { describe, expect, it } from 'vitest';
import {
  browserPersistenceRegistry,
  clearBrowserPreference,
  readBrowserPreference,
  readBrowserSessionPreference,
  writeBrowserPreference,
  writeBrowserSessionPreference
} from './browserStorage';

class MemoryStorage {
  values = new Map<string, string>();

  getItem(key: string) {
    return this.values.get(key) ?? null;
  }

  setItem(key: string, value: string) {
    this.values.set(key, value);
  }

  removeItem(key: string) {
    this.values.delete(key);
  }
}

const isBoolean = (value: unknown): value is boolean => typeof value === 'boolean';

describe('browser preference storage', () => {
  it('round-trips registered local values under a versioned app namespace', () => {
    const storage = new MemoryStorage();
    const key = browserPersistenceRegistry.commonTagsCollapsed.key;
    expect(writeBrowserPreference(key, true, storage)).toBe(true);
    expect(storage.values.get('gooru.preference.v1.common-tags.collapsed')).toBe('true');
    expect(readBrowserPreference(key, false, isBoolean, storage)).toBe(true);
  });

  it('falls back for missing, malformed, or type-invalid local values', () => {
    const storage = new MemoryStorage();
    const key = browserPersistenceRegistry.commonTagsCollapsed.key;
    expect(readBrowserPreference(key, true, isBoolean, storage)).toBe(true);

    storage.values.set('gooru.preference.v1.common-tags.collapsed', '{');
    expect(readBrowserPreference(key, false, isBoolean, storage)).toBe(false);

    storage.values.set('gooru.preference.v1.common-tags.collapsed', JSON.stringify('yes'));
    expect(readBrowserPreference(key, false, isBoolean, storage)).toBe(false);
  });

  it('round-trips registered session values without changing their existing storage key', () => {
    const storage = new MemoryStorage();
    const key = browserPersistenceRegistry.viewerSessionPreferences.key;
    const isViewerState = (value: unknown): value is { version: number } =>
      Boolean(value) && typeof value === 'object' && (value as { version?: unknown }).version === 1;

    expect(writeBrowserSessionPreference(key, { version: 1 }, storage)).toBe(true);
    expect(storage.values.get('gooru.viewer.preferences.v1')).toBe('{"version":1}');
    expect(readBrowserSessionPreference(key, { version: 0 }, isViewerState, storage)).toEqual({ version: 1 });
  });

  it('clears values and treats unavailable or failing storage as best-effort', () => {
    const storage = new MemoryStorage();
    const key = browserPersistenceRegistry.commonTagsCollapsed.key;
    writeBrowserPreference(key, true, storage);
    expect(clearBrowserPreference(key, storage)).toBe(true);
    expect(readBrowserPreference(key, false, isBoolean, storage)).toBe(false);

    const failingStorage = {
      getItem: () => { throw new Error('blocked'); },
      setItem: () => { throw new Error('blocked'); },
      removeItem: () => { throw new Error('blocked'); }
    };
    expect(readBrowserPreference(key, true, isBoolean, failingStorage)).toBe(true);
    expect(writeBrowserPreference(key, true, failingStorage)).toBe(false);
    expect(clearBrowserPreference(key, failingStorage)).toBe(false);
  });
});
