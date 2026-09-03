import { describe, expect, it } from 'vitest';
import { clearBrowserPreference, readBrowserPreference, writeBrowserPreference } from './browserStorage';

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
  it('round-trips typed values under a versioned app namespace', () => {
    const storage = new MemoryStorage();
    expect(writeBrowserPreference('common-tags.collapsed', true, storage)).toBe(true);
    expect(storage.values.get('gooru.preference.v1.common-tags.collapsed')).toBe('true');
    expect(readBrowserPreference('common-tags.collapsed', false, isBoolean, storage)).toBe(true);
  });

  it('falls back for missing, malformed, or type-invalid values', () => {
    const storage = new MemoryStorage();
    expect(readBrowserPreference('missing', true, isBoolean, storage)).toBe(true);

    storage.values.set('gooru.preference.v1.bad-json', '{');
    expect(readBrowserPreference('bad-json', false, isBoolean, storage)).toBe(false);

    storage.values.set('gooru.preference.v1.wrong-type', JSON.stringify('yes'));
    expect(readBrowserPreference('wrong-type', false, isBoolean, storage)).toBe(false);
  });

  it('clears values and treats unavailable or failing storage as best-effort', () => {
    const storage = new MemoryStorage();
    writeBrowserPreference('sidebar.compact', true, storage);
    expect(clearBrowserPreference('sidebar.compact', storage)).toBe(true);
    expect(readBrowserPreference('sidebar.compact', false, isBoolean, storage)).toBe(false);

    const failingStorage = {
      getItem: () => { throw new Error('blocked'); },
      setItem: () => { throw new Error('blocked'); },
      removeItem: () => { throw new Error('blocked'); }
    };
    expect(readBrowserPreference('blocked', true, isBoolean, failingStorage)).toBe(true);
    expect(writeBrowserPreference('blocked', true, failingStorage)).toBe(false);
    expect(clearBrowserPreference('blocked', failingStorage)).toBe(false);
  });
});
