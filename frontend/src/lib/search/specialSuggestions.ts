export type SpecialSearchSuggestion = {
  commit: string;
  ns: string;
  val: string;
  hint: string;
  partial?: boolean;
};

const mediaKinds = ['photo', 'video', 'gif', 'audio', 'other'] as const;

export function specialSearchSuggestions(draft: string, existingTokens: string[] = []): SpecialSearchSuggestion[] {
  let working = draft.trim();
  if (!working) return [];

  let negPrefix = '';
  if (working.startsWith('-')) {
    negPrefix = '-';
    working = working.slice(1);
  }

  const existing = new Set(existingTokens);
  const lower = working.toLowerCase();

  if (lower.startsWith('type:')) {
    const fragment = lower.slice('type:'.length);
    return mediaKinds
      .filter((kind) => !fragment || kind.startsWith(fragment))
      .map((kind) => ({
        commit: `${negPrefix}type:${kind}`,
        ns: 'type',
        val: kind,
        hint: 'media type'
      }))
      .filter((item) => !existing.has(item.commit));
  }

  const result: SpecialSearchSuggestion[] = [];
  if ('@tagged'.startsWith(lower)) {
    const commit = `${negPrefix}@tagged`;
    if (!existing.has(commit)) result.push({ commit, ns: '', val: '@tagged', hint: 'has tags' });
  }
  if ('type:'.startsWith(lower)) {
    const commit = `${negPrefix}type:`;
    result.push({ commit, ns: 'type', val: '', hint: 'media type', partial: true });
  }
  return result;
}
