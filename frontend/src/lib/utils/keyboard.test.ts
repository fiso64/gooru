import { describe, expect, it } from 'vitest';
import { isEditableShortcutTarget, isInteractiveShortcutTarget, libraryShortcutAction } from './keyboard';

type FakeNode = {
  tagName?: string;
  isContentEditable?: boolean;
  controls?: boolean;
  parentElement?: FakeNode | null;
  role?: string;
  type?: string;
  getAttribute?: (name: string) => string | null;
};

function node(properties: Omit<FakeNode, 'getAttribute'>): EventTarget {
  const value: FakeNode = { ...properties };
  value.getAttribute = (name) => {
    if (name === 'role') return value.role ?? null;
    if (name === 'type') return value.type ?? null;
    return null;
  };
  return value as unknown as EventTarget;
}

describe('global shortcut target policy', () => {
  it('protects text-editing controls without treating every input as an editor', () => {
    expect(isEditableShortcutTarget(node({ tagName: 'input' }))).toBe(true);
    expect(isEditableShortcutTarget(node({ tagName: 'input', type: 'search' }))).toBe(true);
    expect(isEditableShortcutTarget(node({ tagName: 'input', type: 'checkbox' }))).toBe(false);
    expect(isEditableShortcutTarget(node({ tagName: 'input', type: 'range' }))).toBe(false);
    expect(isEditableShortcutTarget(node({ tagName: 'span', parentElement: { tagName: 'div', isContentEditable: true } }))).toBe(true);
    expect(isEditableShortcutTarget(node({ tagName: 'button' }))).toBe(false);
  });

  it('still recognizes non-editing inputs and other native controls as interactive', () => {
    expect(isInteractiveShortcutTarget(node({ tagName: 'input', type: 'checkbox' }))).toBe(true);
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
