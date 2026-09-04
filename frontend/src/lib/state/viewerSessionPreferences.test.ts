import { describe, expect, it } from 'vitest';
import {
  readViewerSessionPreferences,
  updateViewerSessionPreferences,
  viewerSessionStorageKey,
  type ViewerSessionPreferences
} from './viewerSessionPreferences';

function memoryStorage(initial?: string) {
  const values = new Map<string, string>();
  if (initial !== undefined) values.set(viewerSessionStorageKey, initial);
  return {
    getItem(key: string) {
      return values.get(key) ?? null;
    },
    setItem(key: string, value: string) {
      values.set(key, value);
    },
    value() {
      return values.get(viewerSessionStorageKey);
    }
  };
}

const defaults: ViewerSessionPreferences = {
  preferOriginal: true,
  rotation: 0,
  fitMode: 'screen'
};

describe('viewer session preferences', () => {
  it('uses runtime defaults when no saved preference exists', () => {
    expect(readViewerSessionPreferences(defaults, memoryStorage())).toEqual(defaults);
  });

  it('restores only validated allowlisted values', () => {
    const storage = memoryStorage(JSON.stringify({
      version: 1,
      preferOriginal: false,
      rotation: 270,
      fitMode: 'actual',
      zoom: 8,
      fileID: 'sensitive-file-id'
    }));

    expect(readViewerSessionPreferences(defaults, storage)).toEqual({
      preferOriginal: false,
      rotation: 270,
      fitMode: 'actual'
    });
  });

  it('ignores malformed, stale, and invalid saved values', () => {
    expect(readViewerSessionPreferences(defaults, memoryStorage('{broken'))).toEqual(defaults);
    expect(readViewerSessionPreferences(defaults, memoryStorage(JSON.stringify({ version: 2, rotation: 90 })))).toEqual(defaults);
    expect(readViewerSessionPreferences(defaults, memoryStorage(JSON.stringify({ version: 1, rotation: 45, fitMode: 'stretch' })))).toEqual(defaults);
  });

  it('updates preferences sparsely so unrelated runtime defaults are not persisted accidentally', () => {
    const storage = memoryStorage();
    updateViewerSessionPreferences({ rotation: 90 }, storage);

    expect(JSON.parse(storage.value() ?? '{}')).toEqual({ version: 1, rotation: 90 });
    expect(readViewerSessionPreferences(defaults, storage)).toEqual({
      preferOriginal: true,
      rotation: 90,
      fitMode: 'screen'
    });
  });

  it('preserves existing saved preferences when a new preference is updated', () => {
    const storage = memoryStorage(JSON.stringify({ version: 1, preferOriginal: false, rotation: 180 }));
    updateViewerSessionPreferences({ fitMode: 'actual' }, storage);

    expect(readViewerSessionPreferences(defaults, storage)).toEqual({
      preferOriginal: false,
      rotation: 180,
      fitMode: 'actual'
    });
  });
});
