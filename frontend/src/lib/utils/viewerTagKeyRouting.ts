import { hasCommandModifier } from './keyboard';

/** Printable characters (including punctuation) belong to the editor, not viewer shortcuts. */
export function isViewerTagTextKey(key: string): boolean {
  return (key.length === 1 && key !== ' ') || key === 'Dead' || key === 'Process' || key === 'Unidentified';
}

/** Only a focused empty tag input for the current viewer file yields non-text keys. */
export function isEmptyViewerTagShortcut(event: KeyboardEvent, fileID: string): boolean {
  if (event.defaultPrevented || event.isComposing || hasCommandModifier(event) || isViewerTagTextKey(event.key)) return false;
  return event.target instanceof HTMLInputElement && event.target.id === `tags-${fileID}` && event.target.value === '';
}
