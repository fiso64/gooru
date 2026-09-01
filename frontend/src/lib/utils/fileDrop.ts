export function hasDraggedFiles(types: ArrayLike<string> | Iterable<string> | null | undefined): boolean {
  return Array.from(types ?? []).includes('Files');
}
