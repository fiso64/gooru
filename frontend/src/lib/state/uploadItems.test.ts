import { describe, expect, it } from 'vitest';
import { effectiveUploadTargetID } from './uploadItems';

const targets = [
  { id: 'primary', name: 'Primary' },
  { id: 'archive', name: 'Archive' }
];

describe('effectiveUploadTargetID', () => {
  it('uses the first configured target when no explicit target is selected', () => {
    expect(effectiveUploadTargetID('', targets)).toBe('primary');
  });

  it('preserves an explicit configured target', () => {
    expect(effectiveUploadTargetID('archive', targets)).toBe('archive');
  });

  it('falls back when a previously selected target disappears', () => {
    expect(effectiveUploadTargetID('removed', targets)).toBe('primary');
  });

  it('returns empty when uploads have no configured targets', () => {
    expect(effectiveUploadTargetID('', [])).toBe('');
  });
});
