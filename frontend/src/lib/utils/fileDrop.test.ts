import { describe, expect, it } from 'vitest';
import { hasDraggedFiles } from './fileDrop';

describe('hasDraggedFiles', () => {
  it('accepts operating-system file drags', () => {
    expect(hasDraggedFiles(['text/plain', 'Files'])).toBe(true);
  });

  it('ignores text/link drags and missing transfer data', () => {
    expect(hasDraggedFiles(['text/plain', 'text/uri-list'])).toBe(false);
    expect(hasDraggedFiles(null)).toBe(false);
  });
});
