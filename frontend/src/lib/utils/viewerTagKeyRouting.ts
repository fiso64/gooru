import { matchesShortcutModifiers } from './keyboard';

/** Printable characters (including punctuation) belong to the editor, not viewer shortcuts. */
export function isViewerTagTextKey(key: string): boolean {
  return (key.length === 1 && key !== ' ') || key === 'Dead' || key === 'Process' || key === 'Unidentified';
}

/** Only a focused empty tag input for the current viewer file yields non-text keys. */
export function isEmptyViewerTagShortcut(event: KeyboardEvent, fileID: string): boolean {
  // Only explicitly defined shifted viewer actions may leave an empty editor.
  // Shift+Escape/Tab/Space and other modified keys keep native input ownership.
  const shiftedViewerAction = matchesShortcutModifiers(event, { shift: true })
    && (event.key === 'ArrowLeft' || event.key === 'ArrowRight' || event.key === 'Delete');
  if (event.defaultPrevented || event.isComposing || !(matchesShortcutModifiers(event) || shiftedViewerAction) || isViewerTagTextKey(event.key)) return false;
  return event.target instanceof HTMLInputElement && event.target.id === `tags-${fileID}` && event.target.value === '';
}
