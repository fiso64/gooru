import { describe, expect, it } from 'vitest';
import { isEditableShortcutTarget, isInteractiveShortcutTarget, libraryShortcutAction } from './keyboard';

type FakeNode = {
  tagName?: string;
  isContentEditable?: boolean;
  controls?: boolean;
  parentElement?: FakeNode | null;
  role?: string;
  getAttribute?: (name: string) => string | null;
};

function node(properties: Omit<FakeNode, 'getAttribute'>): EventTarget {
  const value: FakeNode = { ...properties };
  value.getAttribute = (name) => (name === 'role' ? value.role ?? null : null);
  return value as unknown as EventTarget;
}

describe('global shortcut target policy', () => {
  it('treats text controls and descendants of editable content as editing targets', () => {
    expect(isEditableShortcutTarget(node({ tagName: 'input' }))).toBe(true);
    expect(isEditableShortcutTarget(node({ tagName: 'span', parentElement: { tagName: 'div', isContentEditable: true } }))).toBe(true);
    expect(isEditableShortcutTarget(node({ tagName: 'button' }))).toBe(false);
  });

  it('protects focused controls from media/global shortcuts', () => {
    expect(isInteractiveShortcutTarget(node({ tagName: 'span', parentElement: { tagName: 'button' } }))).toBe(true);
    expect(isInteractiveShortcutTarget(node({ tagName: 'div', role: 'slider' }))).toBe(true);
    expect(isInteractiveShortcutTarget(node({ tagName: 'audio', controls: true }))).toBe(true);
    expect(isInteractiveShortcutTarget(node({ tagName: 'video', controls: false }))).toBe(false);
    expect(isInteractiveShortcutTarget(node({ tagName: 'div' }))).toBe(false);
  });

  it('maps library action shortcuts without enabling tag actions for an empty selection', () => {
    expect(libraryShortcutAction('a', 0)).toBe('select-all');
    expect(libraryShortcutAction('A', 3)).toBe('select-all');
    expect(libraryShortcutAction('t', 2)).toBe('tag-selected');
    expect(libraryShortcutAction('u', 2)).toBe('untag-selected');
    expect(libraryShortcutAction('t', 0)).toBeNull();
    expect(libraryShortcutAction('u', 0)).toBeNull();
  });
});
