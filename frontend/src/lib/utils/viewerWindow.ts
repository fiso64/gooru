// The metadata window is independent of the library grid's loaded transport pages.
export interface ViewerWindow<T extends { id: string }> {
  anchor: string;
  context: string;
  complete?: boolean; // Every other matching file is known; rotate without refetching.
  before: T[]; // nearest first, with global wraparound
  after: T[];  // nearest first, with global wraparound
}

export function windowNeighbor<T extends { id: string }>(window: ViewerWindow<T>, direction: number): T | undefined {
  return (direction < 0 ? window.before : window.after)[0];
}

export function advanceViewerWindow<T extends { id: string }>(window: ViewerWindow<T>, current: T, direction: number): ViewerWindow<T> {
  const next = windowNeighbor(window, direction);
  if (!next) return window;
  if (window.complete) {
    // Each side lists the entire small circular result set. Preserve that
    // complete sequence on every move rather than draining it as a page.
    if (direction < 0) {
      return {
        ...window, anchor: next.id,
        before: [...window.before.slice(1), current],
        after: [current, ...window.after.slice(0, -1)]
      };
    }
    return {
      ...window, anchor: next.id,
      before: [current, ...window.before.slice(0, -1)],
      after: [...window.after.slice(1), current]
    };
  }
  // A short circular listing can appear on both sides of the anchor. Do not
  // accidentally insert the new anchor into its own neighbor list while sliding.
  const distinct = (items: T[]) => {
    const ids = new Set<string>([next.id]);
    return items.filter((item) => {
      if (ids.has(item.id)) return false;
      ids.add(item.id);
      return true;
    }).slice(0, 20);
  };
  if (direction < 0) {
    return { ...window, anchor: next.id, before: distinct(window.before.slice(1)), after: distinct([current, ...window.after]) };
  }
  return { ...window, anchor: next.id, before: distinct([current, ...window.before]), after: distinct(window.after.slice(1)) };
}
