export type PreviewItem = { id: string };

export function previewNeighbor<T extends PreviewItem>(active: T | null, items: T[], delta: number): T | null {
  if (!active || !items.length) return null;
  const index = items.findIndex((item) => item.id === active.id);
  if (index < 0) return null;
  return items[(index + delta + items.length) % items.length] ?? null;
}

/** Prefer the preceding file after removal; unlike normal navigation, never wrap to the removed file. */
export function previewAfterRemoval<T extends PreviewItem>(removedID: string, items: T[]): T | null {
  const index = items.findIndex((item) => item.id === removedID);
  if (index < 0) return null;
  return items[index - 1] ?? items[index + 1] ?? null;
}
