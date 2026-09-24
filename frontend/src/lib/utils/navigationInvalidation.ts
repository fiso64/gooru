// Shared mutation-to-viewer notification; grid queries and the viewer window are independent.
const listeners = new Set<() => void>();

export function onNavigationInvalidated(listener: () => void): () => void {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
}

export function invalidateNavigationAfterMutation(): void {
  for (const listener of listeners) listener();
}
