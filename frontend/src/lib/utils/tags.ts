import type { TagMutationOperation } from '$lib/api/types';

export function applyTagOperation(current: string[], operation: TagMutationOperation, changed: string[]) {
  if (operation === 'set') return Array.from(new Set(changed));

  const next = new Set(current);
  if (operation === 'add') {
    for (const tag of changed) next.add(tag);
  } else {
    for (const tag of changed) next.delete(tag);
  }
  return Array.from(next);
}
