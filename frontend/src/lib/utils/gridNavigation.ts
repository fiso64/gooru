export type GridDirection = 'ArrowLeft' | 'ArrowRight' | 'ArrowUp' | 'ArrowDown';

export type GridRect = {
  left: number;
  top: number;
  width: number;
  height: number;
};

export interface GridNavigationOptions {
  wrapHorizontal?: boolean;
}

export function nextGridIndex(
  rects: GridRect[],
  currentIndex: number,
  direction: GridDirection,
  options: GridNavigationOptions = {}
): number {
  const current = rects[currentIndex];
  if (!current) return currentIndex;

  const currentX = current.left + current.width / 2;
  const currentY = current.top + current.height / 2;
  let bestIndex = currentIndex;
  let bestScore = Number.POSITIVE_INFINITY;

  for (let index = 0; index < rects.length; index += 1) {
    if (index === currentIndex) continue;
    const rect = rects[index];
    const x = rect.left + rect.width / 2;
    const y = rect.top + rect.height / 2;
    const dx = x - currentX;
    const dy = y - currentY;

    let major = 0;
    let minor = 0;
    if (direction === 'ArrowLeft' && dx < -1) {
      major = -dx;
      minor = Math.abs(dy);
    } else if (direction === 'ArrowRight' && dx > 1) {
      major = dx;
      minor = Math.abs(dy);
    } else if (direction === 'ArrowUp' && dy < -1) {
      major = -dy;
      minor = Math.abs(dx);
    } else if (direction === 'ArrowDown' && dy > 1) {
      major = dy;
      minor = Math.abs(dx);
    } else {
      continue;
    }

    // Prefer staying in the same visual row/column, then the nearest item in
    // that direction. This works for both fixed media cards and variable-width
    // tag chips without duplicating layout-specific column calculations.
    const score = major + minor * 4;
    if (score < bestScore) {
      bestScore = score;
      bestIndex = index;
    }
  }

  if (bestIndex !== currentIndex) return bestIndex;

  if (options.wrapHorizontal) {
    if (direction === 'ArrowRight' && currentIndex + 1 < rects.length) return currentIndex + 1;
    if (direction === 'ArrowLeft' && currentIndex > 0) return currentIndex - 1;
  }

  return currentIndex;
}

export function isGridDirection(key: string): key is GridDirection {
  return key === 'ArrowLeft' || key === 'ArrowRight' || key === 'ArrowUp' || key === 'ArrowDown';
}
