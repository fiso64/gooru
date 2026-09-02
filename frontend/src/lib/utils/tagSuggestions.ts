import { compareCompletionRank } from './completionRanking';

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
  kind: 'tag' | 'namespace';
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
  const seenTags = new Map<string, PlainTagSuggestion>();
  const seenNamespaces = new Map<string, PlainTagSuggestion>();

  for (const candidate of candidates) {
    const name = candidateTagName(candidate).trim();
    if (!name || name.startsWith('@') || name.startsWith('-')) continue;

    const separator = name.indexOf(':');
    const namespace = candidate.namespace?.trim() || (separator > 0 ? name.slice(0, separator) : '');
    if (namespace) {
      const namespaceName = `${namespace}:`;
      const previous = seenNamespaces.get(namespaceName);
      seenNamespaces.set(namespaceName, {
        name: namespaceName,
        count: Math.max(candidate.count ?? 0, previous?.count ?? 0),
        kind: 'namespace'
      });
    }

    // Namespace-only candidates are completion prefixes, not concrete tags.
    if (name.endsWith(':') || excluded.has(name.toLowerCase())) continue;
    const previous = seenTags.get(name);
    seenTags.set(name, { name, count: candidate.count ?? previous?.count ?? 0, kind: 'tag' });
  }

  const colon = query.indexOf(':');
  const namespace = colon >= 0 ? query.slice(0, colon) : '';
  const value = colon >= 0 ? query.slice(colon + 1) : query;

  const tags = [...seenTags.values()]
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
    .sort((a, b) => compareCompletionRank(a.name, b.name, value, a.count, b.count));

  const namespaces = colon >= 0
    ? []
    : [...seenNamespaces.values()]
        .filter(({ name }) => name.toLowerCase().includes(query))
        .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name));

  return [...namespaces, ...tags].slice(0, limit);
}

export function isPlainTag(value: string): boolean {
  const tag = value.trim();
  return Boolean(tag) && !tag.endsWith(':') && !tag.startsWith('@') && !tag.startsWith('-') && !/\s/.test(tag);
}

export function plainTagsFromInput(value: string): string[] {
  return Array.from(new Set(value.split(/\s+/).map((tag) => tag.trim()).filter(isPlainTag)));
}
