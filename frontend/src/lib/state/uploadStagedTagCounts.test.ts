import { describe, expect, it } from 'vitest';
import { createUploadStagedTagCounts } from './uploadStagedTagCounts';

describe('createUploadStagedTagCounts', () => {
  it('counts each staged item once per case-insensitive tag', () => {
    const counts = createUploadStagedTagCounts();
    counts.add(['artist:Alice', 'artist:Alice']);
    counts.add(['ARTIST:ALICE', 'local:only']);

    expect(counts.candidates()).toEqual([
      { name: 'artist:Alice', count: 2 },
      { name: 'local:only', count: 1 }
    ]);
  });

  it('updates only changed tag occurrences and removes zero-count candidates', () => {
    const counts = createUploadStagedTagCounts();
    counts.add(['shared', 'old']);
    counts.add(['shared']);

    counts.replace(['shared', 'old'], ['shared', 'new']);
    expect(counts.candidates()).toEqual([
      { name: 'shared', count: 2 },
      { name: 'new', count: 1 }
    ]);

    counts.remove(['shared', 'new']);
    expect(counts.candidates()).toEqual([{ name: 'shared', count: 1 }]);
    counts.clear();
    expect(counts.candidates()).toEqual([]);
  });
});
