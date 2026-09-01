type ShortcutNode = {
  tagName?: string;
  isContentEditable?: boolean;
  controls?: boolean;
  parentElement?: ShortcutNode | null;
  getAttribute?: (name: string) => string | null;
};

const editableTags = new Set(['INPUT', 'TEXTAREA', 'SELECT']);
const interactiveTags = new Set(['BUTTON', 'A']);
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

/** Returns true when a global shortcut would steal normal text/editing input. */
export function isEditableShortcutTarget(target: EventTarget | null) {
  return shortcutAncestors(target).some((node) => {
    const tag = node.tagName?.toUpperCase();
    return Boolean(node.isContentEditable) || (tag ? editableTags.has(tag) : false);
  });
}

/** Returns true when Space/etc. should be left to the focused native control. */
export function isInteractiveShortcutTarget(target: EventTarget | null) {
  return shortcutAncestors(target).some((node) => {
    const tag = node.tagName?.toUpperCase();
    if (Boolean(node.isContentEditable) || (tag ? editableTags.has(tag) || interactiveTags.has(tag) : false)) return true;
    if ((tag === 'AUDIO' || tag === 'VIDEO') && node.controls) return true;
    const role = node.getAttribute?.('role')?.toLowerCase();
    return role ? interactiveRoles.has(role) : false;
  });
}

export function hasCommandModifier(event: KeyboardEvent) {
  return event.altKey || event.ctrlKey || event.metaKey;
}
