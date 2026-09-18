import { describe, expect, it } from 'vitest';
import { editUploadTags } from './uploadBulkTags';

describe('editUploadTags', () => {
  it('adds unique tags without disturbing existing order', () => {
    expect(editUploadTags(['shared', 'local'], 'add', ['bulk', 'shared'])).toEqual(['shared', 'local', 'bulk']);
  });

  it('removes only requested tags', () => {
    expect(editUploadTags(['shared', 'local', 'bulk'], 'remove', ['shared', 'missing'])).toEqual(['local', 'bulk']);
  });

  it('sets tags exactly and supports empty SET as clear-all', () => {
    expect(editUploadTags(['old'], 'set', ['next', 'next', ''])).toEqual(['next']);
    expect(editUploadTags(['old'], 'set', [])).toEqual([]);
  });
});
