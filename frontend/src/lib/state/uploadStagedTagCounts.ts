export type UploadStagedTagCandidate = { name: string; count: number };

function uniqueTagNames(tags: string[]): Map<string, string> {
  const unique = new Map<string, string>();
  for (const rawTag of tags) {
    const name = rawTag.trim();
    const key = name.toLowerCase();
    if (name && !unique.has(key)) unique.set(key, name);
  }
  return unique;
}

export function createUploadStagedTagCounts() {
  const counts = new Map<string, UploadStagedTagCandidate>();

  function add(tags: string[]) {
    for (const [key, name] of uniqueTagNames(tags)) {
      const current = counts.get(key);
      if (current) current.count += 1;
      else counts.set(key, { name, count: 1 });
    }
  }

  function remove(tags: string[]) {
    for (const key of uniqueTagNames(tags).keys()) {
      const current = counts.get(key);
      if (!current) continue;
      if (current.count <= 1) counts.delete(key);
      else current.count -= 1;
    }
  }

  function replace(previousTags: string[], nextTags: string[]) {
    const previous = uniqueTagNames(previousTags);
    const next = uniqueTagNames(nextTags);
    for (const [key, name] of previous) {
      if (!next.has(key)) remove([name]);
    }
    for (const [key, name] of next) {
      if (!previous.has(key)) add([name]);
    }
  }

  return {
    add,
    remove,
    replace,
    clear: () => counts.clear(),
    candidates: (): UploadStagedTagCandidate[] => Array.from(counts.values(), (candidate) => ({ ...candidate }))
  };
}
