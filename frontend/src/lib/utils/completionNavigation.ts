export function moveCompletionIndex(active: number, count: number, direction: -1 | 1): number {
  return count > 0 ? (active + direction + count) % count : 0;
}
