export type SpecialSearchSuggestion = {
  commit: string;
  ns: string;
  val: string;
  hint: string;
  partial?: boolean;
};

const mediaTypes = ['photo', 'video', 'gif', 'audio', 'other'] as const;

export function specialSearchSuggestions(draft: string, existing: string[] = []): SpecialSearchSuggestion[] {
  let working = draft.trim();
  if (!working) return [];

  let negPrefix = '';
  if (working.startsWith('-')) {
    negPrefix = '-';
    working = working.slice(1);
  }

  const query = working.toLowerCase();
  const taken = new Set(existing.map((value) => value.toLowerCase()));
  const suggestions: SpecialSearchSuggestion[] = [];

  const tagged = `${negPrefix}@tagged`;
  if ('@tagged'.startsWith(query) && !taken.has(tagged.toLowerCase())) {
    suggestions.push({ commit: tagged, ns: '', val: '@tagged', hint: 'expression' });
  }

  if (!query.includes(':')) {
    const typePrefix = `${negPrefix}type:`;
    if ('type:'.startsWith(query) && !taken.has(typePrefix.toLowerCase())) {
      suggestions.push({ commit: typePrefix, ns: 'type', val: '', hint: 'media type', partial: true });
    }
    return suggestions;
  }

  const colon = query.indexOf(':');
  if (query.slice(0, colon) !== 'type') return suggestions;
  const valueFragment = query.slice(colon + 1);
  for (const value of mediaTypes) {
    if (valueFragment && !value.startsWith(valueFragment)) continue;
    const commit = `${negPrefix}type:${value}`;
    if (taken.has(commit.toLowerCase())) continue;
    suggestions.push({ commit, ns: 'type', val: value, hint: 'media type' });
  }
  return suggestions;
}
