import { rankCompletionCandidates } from './completionRanking';

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

export function mergeTagCandidateCounts(candidates: TagCandidate[], tagSets: string[][]): TagCandidate[] {
  const merged = candidates.map((candidate) => ({ ...candidate }));
  const indexByName = new Map<string, number>();
  for (let index = 0; index < merged.length; index += 1) {
    const name = candidateTagName(merged[index]!).trim();
    if (name) indexByName.set(name.toLowerCase(), index);
  }

  const stagedCounts = new Map<string, { name: string; count: number }>();
  for (const tags of tagSets) {
    const seen = new Set<string>();
    for (const rawTag of tags) {
      const name = rawTag.trim();
      const key = name.toLowerCase();
      if (!name || seen.has(key)) continue;
      seen.add(key);
      const previous = stagedCounts.get(key);
      stagedCounts.set(key, { name: previous?.name ?? name, count: (previous?.count ?? 0) + 1 });
    }
  }

  for (const [key, staged] of stagedCounts) {
    const index = indexByName.get(key);
    if (index == null) {
      indexByName.set(key, merged.length);
      merged.push({ name: staged.name, count: staged.count });
      continue;
    }
    const candidate = merged[index]!;
    merged[index] = { ...candidate, count: (candidate.count ?? 0) + staged.count };
  }

  return merged;
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

  // The base key count is the unique-file aggregate for both the plain key and
  // its namespace completion. Promote derived namespace counts to that same
  // aggregate when the base candidate is present; a plain-only key still never
  // creates a namespace candidate.
  for (const [namespaceName, item] of seenNamespaces) {
    const base = seenTags.get(namespaceName.slice(0, -1));
    if (base) seenNamespaces.set(namespaceName, { ...item, count: base.count });
  }

  const colon = query.indexOf(':');
  const namespace = colon >= 0 ? query.slice(0, colon) : '';
  const value = colon >= 0 ? query.slice(colon + 1) : query;

  const tags = [...seenTags.values()].filter(({ name }) => {
    const lower = name.toLowerCase();
    const nameColon = lower.indexOf(':');
    if (colon >= 0) {
      if (nameColon < 0 || lower.slice(0, nameColon) !== namespace) return false;
      return !value || lower.slice(nameColon + 1).includes(value);
    }
    const candidateValue = nameColon >= 0 ? lower.slice(nameColon + 1) : lower;
    return lower.includes(query) || candidateValue.includes(query);
  });

  if (colon >= 0) {
    return rankCompletionCandidates(tags, value).slice(0, limit);
  }

  const namespaces = [...seenNamespaces.values()].filter(({ name }) => name.toLowerCase().includes(query));

  // Rank all visible completion kinds together. Category-specific pre-sorting or
  // prepending lets surfaces drift even when they share the same comparator.
  return rankCompletionCandidates([...namespaces, ...tags], query).slice(0, limit);
}

export function isPlainTag(value: string): boolean {
  const tag = value.trim();
  return Boolean(tag) && !tag.endsWith(':') && !tag.startsWith('@') && !tag.startsWith('-') && !/\s/.test(tag);
}

export function plainTagsFromInput(value: string): string[] {
  return Array.from(new Set(value.split(/\s+/).map((tag) => tag.trim()).filter(isPlainTag)));
}
