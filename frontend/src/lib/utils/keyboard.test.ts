import { describe, expect, it } from 'vitest';
import { isEditableShortcutTarget, isInteractiveShortcutTarget, libraryShortcutAction, matchesShortcut, matchesShortcutCode, matchesShortcutModifiers, searchShortcutAction, uploadShortcutAction } from './keyboard';

type FakeNode = {
  tagName?: string;
  isContentEditable?: boolean;
  controls?: boolean;
  parentElement?: FakeNode | null;
  role?: string;
  ariaModal?: string;
  type?: string;
  getAttribute?: (name: string) => string | null;
};

function node(properties: Omit<FakeNode, 'getAttribute'>): EventTarget {
  const value: FakeNode = { ...properties };
  value.getAttribute = (name) => {
    if (name === 'role') return value.role ?? null;
    if (name === 'type') return value.type ?? null;
    if (name === 'aria-modal') return (value as FakeNode & { ariaModal?: string }).ariaModal ?? null;
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

  it('scopes search shortcuts away from editors, modal dialogs, modifiers, and shifted keys', () => {
    const modal = { tagName: 'div', getAttribute: (name: string) => name === 'role' ? 'dialog' : name === 'aria-modal' ? 'true' : null } as FakeNode;
    expect(searchShortcutAction('/', node({ tagName: 'div' }))).toBe('focus-search');
    expect(searchShortcutAction('f', node({ tagName: 'button' }))).toBe('filename-search');
    expect(searchShortcutAction('/', node({ tagName: 'input' }))).toBeNull();
    expect(searchShortcutAction('f', node({ tagName: 'span', parentElement: modal }))).toBeNull();
    expect(searchShortcutAction('f', node({ tagName: 'div' }), true)).toBeNull();
    expect(searchShortcutAction('f', node({ tagName: 'div' }), false, true)).toBeNull();
  });

  it('routes ctrl+enter to upload except when a modal owns the event', () => {
    const modal = { tagName: 'div', getAttribute: (name: string) => name === 'role' ? 'dialog' : name === 'aria-modal' ? 'true' : null } as FakeNode;
    expect(uploadShortcutAction('Enter', node({ tagName: 'div' }), true)).toBe('submit-upload');
    expect(uploadShortcutAction('Enter', node({ tagName: 'input' }), true)).toBe('submit-upload');
    expect(uploadShortcutAction('Enter', node({ tagName: 'span', parentElement: modal }), true)).toBeNull();
    expect(uploadShortcutAction('Enter', node({ tagName: 'div' }))).toBeNull();
    expect(uploadShortcutAction('Enter', node({ tagName: 'div' }), true, true)).toBeNull();
    expect(uploadShortcutAction('Enter', node({ tagName: 'div' }), true, false, true)).toBeNull();
    expect(uploadShortcutAction('Enter', node({ tagName: 'div' }), true, false, false, true)).toBeNull();
  });

  it('maps library action shortcuts without enabling tag actions for an empty selection', () => {
    expect(libraryShortcutAction('a', { selectedCount: 0, cursorAvailable: false })).toBe('select-all');
    expect(libraryShortcutAction('A', { selectedCount: 3, cursorAvailable: false })).toBe('select-all');
    expect(libraryShortcutAction('a', { selectedCount: 0, cursorAvailable: false, ctrlKey: true })).toBe('select-all');
    expect(libraryShortcutAction('a', { selectedCount: 0, cursorAvailable: false, metaKey: true })).toBeNull();
    expect(libraryShortcutAction('d', { selectedCount: 2, cursorAvailable: false, ctrlKey: true })).toBeNull();
    expect(libraryShortcutAction('d', { selectedCount: 2, cursorAvailable: false })).toBe('download-selected');
    expect(libraryShortcutAction('D', { selectedCount: 2, cursorAvailable: false })).toBe('download-selected');
    expect(libraryShortcutAction('t', { selectedCount: 2, cursorAvailable: false })).toBe('tag-selected');
    expect(libraryShortcutAction('u', { selectedCount: 2, cursorAvailable: false })).toBe('untag-selected');
    expect(libraryShortcutAction('Delete', { selectedCount: 2, cursorAvailable: false })).toBe('untrack-selected');
    expect(libraryShortcutAction('Delete', { selectedCount: 2, cursorAvailable: false, shiftKey: true })).toBe('delete-selected');
    expect(libraryShortcutAction('d', { selectedCount: 0, cursorAvailable: false })).toBeNull();
    expect(libraryShortcutAction('t', { selectedCount: 0, cursorAvailable: false })).toBeNull();
    expect(libraryShortcutAction('u', { selectedCount: 0, cursorAvailable: false })).toBeNull();
    expect(libraryShortcutAction('Delete', { selectedCount: 0, cursorAvailable: false })).toBeNull();
  });
});

describe('exact keyboard shortcut modifiers', () => {
  const combinations = Array.from({ length: 16 }, (_, bits) => ({
    shiftKey: Boolean(bits & 1), altKey: Boolean(bits & 2),
    ctrlKey: Boolean(bits & 4), metaKey: Boolean(bits & 8)
  }));
  const event = (key: string, code: string, modifiers: (typeof combinations)[number]) =>
    ({ key, code, ...modifiers }) as KeyboardEvent;

  it('recognizes a plain key only without any of the four modifiers', () => {
    for (const [bits, combination] of combinations.entries()) {
      const arrow = event('ArrowLeft', 'ArrowLeft', combination);
      expect(matchesShortcut(arrow, 'ArrowLeft'), `modifier bitmask ${bits}`).toBe(bits === 0);
      expect(matchesShortcut(arrow, 'ArrowRight')).toBe(false);
      expect(matchesShortcutModifiers(arrow)).toBe(bits === 0);
    }
  });

  it('requires exactly the explicitly requested combination, rejecting added modifiers', () => {
    const cases = [
      { mask: 1, key: 'ArrowLeft', modifiers: { shift: true } },
      { mask: 2, key: 'Enter', modifiers: { alt: true } },
      { mask: 4, key: 'a', modifiers: { ctrl: true } },
      { mask: 8, key: 'Enter', modifiers: { meta: true } },
      { mask: 3, key: 'ArrowRight', modifiers: { shift: true, alt: true } }
    ] as const;
    for (const { mask, key, modifiers } of cases) {
      for (const [bits, combination] of combinations.entries()) {
        expect(matchesShortcut(event(key, 'Key', combination), key, modifiers),
          `${key}: expected ${mask}, received ${bits}`).toBe(bits === mask);
      }
    }
  });

  it('matches the physical Space key only with its declared modifiers', () => {
    for (const [bits, combination] of combinations.entries()) {
      expect(matchesShortcutCode(event(' ', 'Space', combination), 'Space')).toBe(bits === 0);
    }
    expect(matchesShortcutCode(event('a', 'KeyA', combinations[0]), 'Space')).toBe(false);
    expect(matchesShortcut(event('A', 'KeyA', combinations[0]), 'a')).toBe(true); // Caps Lock
  });

  it('never treats Shift+letter or an extra modifier as the unmodified library action', () => {
    for (const [bits, combination] of combinations.entries()) {
      for (const key of ['a', 'd', 't', 'u', 'Delete']) {
        const context = { selectedCount: 2, cursorAvailable: true, ...combination };
        if (bits === 0) expect(libraryShortcutAction(key, context)).not.toBeNull();
        else if (key === 'Delete' && bits === 1) expect(libraryShortcutAction(key, context)).toBe('delete-selected');
        else if (key === 'a' && bits === 4) expect(libraryShortcutAction(key, context)).toBe('select-all');
        else expect(libraryShortcutAction(key, context), `${key}: modifier bitmask ${bits}`).toBeNull();
      }
    }
  });
});
