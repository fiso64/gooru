import { describe, expect, it } from 'vitest';
import { isViewerTagTextKey } from './viewerTagKeyRouting';

describe('viewer tag keyboard ownership', () => {
  it('leaves printable text including punctuation and composition keys in the editor', () => {
    for (const key of ['a', 'Z', '4', '(', ')', ',', '.', '?', '!', '+', '-', '_', ':', 'é', 'Dead', 'Process', 'Unidentified']) {
      expect(isViewerTagTextKey(key), key).toBe(true);
    }
  });

  it('delegates non-text keys including Space to the viewer', () => {
    for (const key of [' ', 'ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Delete', 'Backspace', 'Enter', 'Escape', 'Tab']) {
      expect(isViewerTagTextKey(key), key).toBe(false);
    }
  });
});
