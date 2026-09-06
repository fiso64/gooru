import { describe, expect, it } from 'vitest';
import {
  applySelectionMembership,
  applySelectionSnapshot,
  emptySelection,
  selectAllMatching,
  selectionActive,
  selectionCount,
  selectionHas,
  selectionRequest,
  selectionUnknownIDs,
  setSelectionRange,
  toggleSelection
} from './selection';

describe('library selection', () => {
  it('tracks explicit selections without changing unrelated files', () => {
    let selection = emptySelection();
    selection = toggleSelection(selection, 'a');
    expect(selectionHas(selection, 'a')).toBe(true);
    expect(selectionHas(selection, 'b')).toBe(false);
    expect(selectionCount(selection)).toBe(1);
    expect(selectionRequest(selection)).toEqual({ file_ids: ['a'] });
  });

  it('enters select-all immediately before the snapshot returns', () => {
    const selection = selectAllMatching('', 42, ['a', 'b'], 7);
    expect(selectionActive(selection)).toBe(true);
    expect(selectionCount(selection)).toBe(42);
    expect(selectionHas(selection, 'a')).toBe(true);
    expect(selectionHas(selection, 'b')).toBe(true);
    expect(() => selectionRequest(selection)).toThrow('still loading');
  });

  it('hydrates an immutable snapshot count and selector', () => {
    let selection = selectAllMatching('type:video', 8, ['a', 'b'], 4);
    selection = applySelectionSnapshot(selection, 4, 'snapshot-one', 11);
    expect(selectionCount(selection)).toBe(11);
    expect(selectionRequest(selection)).toEqual({ selection_id: 'snapshot-one' });
  });

  it('keeps deselected snapshot members as exclusions', () => {
    let selection = selectAllMatching('type:video', 8, ['a', 'skip-me'], 4);
    selection = applySelectionSnapshot(selection, 4, 'snapshot-one', 8);
    selection = applySelectionMembership(selection, 'snapshot-one', ['a', 'skip-me'], ['a', 'skip-me']);
    selection = toggleSelection(selection, 'skip-me');
    expect(selectionHas(selection, 'skip-me')).toBe(false);
    expect(selectionCount(selection)).toBe(7);
    expect(selectionRequest(selection)).toEqual({ selection_id: 'snapshot-one', exclude_file_ids: ['skip-me'] });
  });

  it('treats newly encountered non-members as explicit additions', () => {
    let selection = selectAllMatching('hidden', 3, ['a', 'b', 'c'], 2);
    selection = applySelectionSnapshot(selection, 2, 'snapshot-one', 3);
    expect(selectionUnknownIDs(selection, ['a', 'new'])).toEqual(['a', 'new']);
    selection = applySelectionMembership(selection, 'snapshot-one', ['a', 'new'], ['a']);
    expect(selectionHas(selection, 'new')).toBe(false);
    selection = toggleSelection(selection, 'new');
    expect(selectionHas(selection, 'new')).toBe(true);
    expect(selectionCount(selection)).toBe(4);
    expect(selectionRequest(selection)).toEqual({ selection_id: 'snapshot-one', include_file_ids: ['new'] });
  });

  it('normalizes optimistic overrides when membership arrives', () => {
    let selection = selectAllMatching('hidden', 2, ['a', 'b'], 2);
    selection = applySelectionSnapshot(selection, 2, 'snapshot-one', 2);
    selection = toggleSelection(selection, 'a');
    expect(selectionCount(selection)).toBe(1);
    selection = applySelectionMembership(selection, 'snapshot-one', ['a', 'b'], ['a']);
    expect(selectionCount(selection)).toBe(1);
    expect(selectionHas(selection, 'b')).toBe(false);
    expect(selectionRequest(selection)).toEqual({ selection_id: 'snapshot-one', exclude_file_ids: ['a'] });
  });

  it('applies select and deselect ranges to explicit and snapshot selections', () => {
    let explicit = setSelectionRange(emptySelection(), ['a', 'b', 'c'], true);
    expect(selectionRequest(explicit)).toEqual({ file_ids: ['a', 'b', 'c'] });
    explicit = setSelectionRange(explicit, ['b', 'c'], false);
    expect(selectionRequest(explicit)).toEqual({ file_ids: ['a'] });

    let snapshot = selectAllMatching('rating:safe', 3, ['a', 'b', 'c'], 1);
    snapshot = applySelectionSnapshot(snapshot, 1, 'snapshot-one', 3);
    snapshot = applySelectionMembership(snapshot, 'snapshot-one', ['a', 'b', 'c'], ['a', 'b', 'c']);
    snapshot = setSelectionRange(snapshot, ['b', 'c'], false);
    snapshot = setSelectionRange(snapshot, ['c'], true);
    expect(selectionRequest(snapshot)).toEqual({ selection_id: 'snapshot-one', exclude_file_ids: ['b'] });
  });
});
