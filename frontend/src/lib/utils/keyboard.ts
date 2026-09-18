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
  if (key !== 'Enter' || !ctrlKey || metaKey || altKey || shiftKey || isModalShortcutTarget(target)) return null;
  return 'submit-upload';
}

export type LibraryShortcutAction = 'select-all' | 'download-selected' | 'tag-selected' | 'untag-selected' | 'untrack-selected' | 'delete-selected' | null;

export function libraryShortcutAction(key: string, selectedCount: number, shiftKey = false): LibraryShortcutAction {
  switch (key.toLowerCase()) {
    case 'a': return 'select-all';
    case 'd': return selectedCount > 0 ? 'download-selected' : null;
    case 't': return selectedCount > 0 ? 'tag-selected' : null;
    case 'u': return selectedCount > 0 ? 'untag-selected' : null;
    case 'delete': return selectedCount > 0 ? (shiftKey ? 'delete-selected' : 'untrack-selected') : null;
    default: return null;
  }
}
