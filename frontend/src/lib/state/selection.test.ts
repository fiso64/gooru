import { describe, expect, it } from 'vitest';
import { emptySelection, selectAllMatching, selectionActive, selectionCount, selectionHas, selectionRequest, toggleSelection } from './selection';

describe('library selection', () => {
  it('tracks explicit selections without changing unrelated files', () => {
    let selection = emptySelection();
    selection = toggleSelection(selection, 'a');
    expect(selectionHas(selection, 'a')).toBe(true);
    expect(selectionHas(selection, 'b')).toBe(false);
    expect(selectionCount(selection, 10)).toBe(1);
    expect(selectionRequest(selection)).toEqual({ file_ids: ['a'] });
  });

  it('represents selecting the root library as a wildcard query', () => {
    const selection = selectAllMatching('');
    expect(selectionActive(selection)).toBe(true);
    expect(selectionCount(selection, 42)).toBe(42);
    expect(selectionRequest(selection)).toEqual({ query: '*' });
  });

  it('keeps deselected query matches as exclusions', () => {
    let selection = selectAllMatching('type:video');
    selection = toggleSelection(selection, 'skip-me');
    expect(selectionHas(selection, 'skip-me')).toBe(false);
    expect(selectionHas(selection, 'keep-me')).toBe(true);
    expect(selectionCount(selection, 8)).toBe(7);
    expect(selectionRequest(selection)).toEqual({ query: 'type:video', exclude_file_ids: ['skip-me'] });
  });
});
