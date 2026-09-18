export interface TagEditDelta {
  add: string[];
  remove: string[];
}

export function tagEditDelta(initialTags: string[], nextTags: string[]): TagEditDelta {
  const initial = new Set(initialTags);
  const next = new Set(nextTags);
  return {
    add: [...next].filter((tag) => !initial.has(tag)),
    remove: [...initial].filter((tag) => !next.has(tag))
  };
}
