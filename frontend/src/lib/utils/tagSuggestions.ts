export type TagCandidate = {
  name?: string;
  tag?: string;
  namespace?: string;
  value?: string;
  count?: number;
};

export type PlainTagSuggestion = {
  name: string;
  count: number;
};

export function candidateTagName(candidate: TagCandidate): string {
  return candidate.name ?? candidate.tag ?? (candidate.namespace ? `${candidate.namespace}:${candidate.value ?? ''}` : (candidate.value ?? ''));
}

export function plainTagSuggestions(
  draft: string,
  candidates: TagCandidate[],
  existing: string[] = [],
  limit = 10
): PlainTagSuggestion[] {
  const query = draft.trim().toLowerCase();
  if (!query || query.startsWith('@') || query.startsWith('-')) return [];

  const excluded = new Set(existing.map((tag) => tag.toLowerCase()));
  const seen = new Map<string, PlainTagSuggestion>();

  for (const candidate of candidates) {
    const name = candidateTagName(candidate).trim();
    if (!name || name.startsWith('@') || name.startsWith('-') || excluded.has(name.toLowerCase())) continue;
    const previous = seen.get(name);
    seen.set(name, { name, count: candidate.count ?? previous?.count ?? 0 });
  }

  const colon = query.indexOf(':');
  const namespace = colon >= 0 ? query.slice(0, colon) : '';
  const value = colon >= 0 ? query.slice(colon + 1) : query;

  return [...seen.values()]
    .filter(({ name }) => {
      const lower = name.toLowerCase();
      const nameColon = lower.indexOf(':');
      if (colon >= 0) {
        if (nameColon < 0 || lower.slice(0, nameColon) !== namespace) return false;
        return !value || lower.slice(nameColon + 1).includes(value);
      }
      const candidateValue = nameColon >= 0 ? lower.slice(nameColon + 1) : lower;
      return lower.includes(query) || candidateValue.includes(query);
    })
    .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name))
    .slice(0, limit);
}

export function isPlainTag(value: string): boolean {
  const tag = value.trim();
  return Boolean(tag) && !tag.startsWith('@') && !tag.startsWith('-') && !/\s/.test(tag);
}
