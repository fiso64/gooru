type ShortcutNode = {
  tagName?: string;
  isContentEditable?: boolean;
  controls?: boolean;
  parentElement?: ShortcutNode | null;
  getAttribute?: (name: string) => string | null;
};

const editableTags = new Set(['TEXTAREA', 'SELECT']);
const nonEditingInputTypes = new Set(['button', 'checkbox', 'color', 'file', 'hidden', 'image', 'radio', 'range', 'reset', 'submit']);
const interactiveTags = new Set(['BUTTON', 'A', 'INPUT']);
const interactiveRoles = new Set(['button', 'checkbox', 'combobox', 'link', 'listbox', 'menuitem', 'option', 'radio', 'slider', 'spinbutton', 'switch', 'tab', 'textbox']);

function shortcutAncestors(target: EventTarget | null) {
  const nodes: ShortcutNode[] = [];
  let node = target as unknown as ShortcutNode | null;
  while (node) {
    nodes.push(node);
    node = node.parentElement ?? null;
  }
  return nodes;
}

function isEditingNode(node: ShortcutNode) {
  const tag = node.tagName?.toUpperCase();
  if (Boolean(node.isContentEditable)) return true;
  if (tag && editableTags.has(tag)) return true;
  if (tag !== 'INPUT') return false;
  const type = node.getAttribute?.('type')?.toLowerCase() ?? 'text';
  return !nonEditingInputTypes.has(type);
}

/** Returns true when a global shortcut would steal normal text/editing input. */
export function isEditableShortcutTarget(target: EventTarget | null) {
  return shortcutAncestors(target).some(isEditingNode);
}

/** Returns true when Space/etc. should be left to the focused native control. */
export function isInteractiveShortcutTarget(target: EventTarget | null) {
  return shortcutAncestors(target).some((node) => {
    const tag = node.tagName?.toUpperCase();
    if (isEditingNode(node) || (tag ? interactiveTags.has(tag) : false)) return true;
    if ((tag === 'AUDIO' || tag === 'VIDEO') && node.controls) return true;
    const role = node.getAttribute?.('role')?.toLowerCase();
    return role ? interactiveRoles.has(role) : false;
  });
}

/** A shortcut owns exactly the modifiers it declares. Omitted flags mean false.
 * Leave unmatched combinations to the browser or another focused control.
 */
export type ShortcutModifiers = {
  shift?: boolean;
  alt?: boolean;
  ctrl?: boolean;
  meta?: boolean;
};

type ShortcutEvent = Pick<KeyboardEvent, 'key' | 'code' | 'shiftKey' | 'altKey' | 'ctrlKey' | 'metaKey'>;

export function matchesShortcutModifiers(event: ShortcutEvent, modifiers: ShortcutModifiers = {}): boolean {
  return event.shiftKey === (modifiers.shift ?? false)
    && event.altKey === (modifiers.alt ?? false)
    && event.ctrlKey === (modifiers.ctrl ?? false)
    && event.metaKey === (modifiers.meta ?? false);
}

/** Use event.key for logical shortcuts. Letters also work under Caps Lock. */
export function matchesShortcut(event: ShortcutEvent, key: string, modifiers: ShortcutModifiers = {}): boolean {
  const matchesKey = /^[a-z]$/.test(key)
    ? event.key.toLowerCase() === key
    : event.key === key;
  return matchesKey && matchesShortcutModifiers(event, modifiers);
}

/** Use event.code only for shortcuts intentionally bound to a physical key. */
export function matchesShortcutCode(event: ShortcutEvent, code: string, modifiers: ShortcutModifiers = {}): boolean {
  return event.code === code && matchesShortcutModifiers(event, modifiers);
}

export function hasCommandModifier(event: KeyboardEvent) {
  return event.altKey || event.ctrlKey || event.metaKey;
}

function isModalShortcutTarget(target: EventTarget | null) {
  return shortcutAncestors(target).some((node) =>
    node.getAttribute?.('role')?.toLowerCase() === 'dialog'
    && node.getAttribute?.('aria-modal')?.toLowerCase() === 'true'
  );
}

function modalShortcutOwnerOpen() {
  if (typeof document === 'undefined') return false;
  return Boolean(document.querySelector('[role="dialog"][aria-modal="true"]'));
}

export type SearchShortcutAction = 'focus-search' | 'filename-search' | null;

export function searchShortcutAction(
  key: string,
  target: EventTarget | null,
  modified = false,
  shiftKey = false,
  modalOpen = modalShortcutOwnerOpen()
): SearchShortcutAction {
  if (modified || shiftKey || modalOpen || isEditableShortcutTarget(target) || isModalShortcutTarget(target)) return null;
  if (key === '/') return 'focus-search';
  if (key.toLowerCase() === 'f') return 'filename-search';
  return null;
}

export type UploadShortcutAction = 'submit-upload' | null;

export function uploadShortcutAction(
  key: string,
  target: EventTarget | null,
  ctrlKey = false,
  metaKey = false,
  altKey = false,
  shiftKey = false
): UploadShortcutAction {
  if (!matchesShortcut({ key, code: '', ctrlKey, metaKey, altKey, shiftKey }, 'Enter', { ctrl: true }) || isModalShortcutTarget(target)) return null;
  return 'submit-upload';
}

export type LibraryShortcutAction = 'select-all' | 'download-selected' | 'tag-selected' | 'untag-selected' | 'untrack-selected' | 'delete-selected' | null;

export interface LibraryShortcutContext {
  selectedCount: number;
  cursorAvailable: boolean;
  shiftKey?: boolean;
  altKey?: boolean;
  ctrlKey?: boolean;
  metaKey?: boolean;
}

export function libraryShortcutAction(key: string, context: LibraryShortcutContext): LibraryShortcutAction {
  const { selectedCount, cursorAvailable, shiftKey = false, altKey = false, ctrlKey = false, metaKey = false } = context;
  const targetAvailable = selectedCount > 0 || cursorAvailable;
  const event = { key, code: '', shiftKey, altKey, ctrlKey, metaKey };
  if (matchesShortcut(event, 'a', { ctrl: true })) return 'select-all';
  if (matchesShortcut(event, 'Enter', { alt: true })) return targetAvailable ? 'tag-selected' : null;
  if (matchesShortcut(event, 'Delete', { shift: true })) return targetAvailable ? 'delete-selected' : null;
  if (!matchesShortcutModifiers(event)) return null;

  switch (key.toLowerCase()) {
    case 'a': return 'select-all';
    case 'd': return targetAvailable ? 'download-selected' : null;
    case 't': return targetAvailable ? 'tag-selected' : null;
    case 'u': return targetAvailable ? 'untag-selected' : null;
    case 'delete': return targetAvailable ? 'untrack-selected' : null;
    default: return null;
  }
}
