import { normalizeUploadItemTags } from './uploadItems';

export type UploadBulkTagOperation = 'add' | 'remove' | 'set';

export function editUploadTags(currentTags: string[], operation: UploadBulkTagOperation, rawOperand: string[]): string[] {
  const current = normalizeUploadItemTags(currentTags);
  const operand = normalizeUploadItemTags(rawOperand);
  if (operation === 'set') return [...operand];
  if (operation === 'add') return normalizeUploadItemTags([...current, ...operand]);
  if (operand.length === 0) return current;
  const removed = new Set(operand);
  return current.filter((tag) => !removed.has(tag));
}
