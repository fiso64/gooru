export type PreviewItem = { id: string };

export function previewNeighbor<T extends PreviewItem>(active: T | null, items: T[], delta: number): T | null {
  if (!active || !items.length) return null;
  const index = items.findIndex((item) => item.id === active.id);
  if (index < 0) return null;
  return items[(index + delta + items.length) % items.length] ?? null;
}
