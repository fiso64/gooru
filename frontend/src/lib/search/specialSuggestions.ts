export type SpecialSearchSuggestion = {
  commit: string;
  ns: string;
  val: string;
  hint: string;
  partial?: boolean;
};

const mediaKinds = ['photo', 'video', 'gif', 'audio', 'other'] as const;

export function specialSearchSuggestions(
  draft: string,
  existingTokens: string[] = [],
  metaTags: Array<{ syntax: string; hint: string; requires_value: boolean }> = []
): SpecialSearchSuggestion[] {
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
  for (const metaTag of metaTags) {
    const syntax = metaTag.syntax.trim();
    if (!syntax.startsWith('@')) continue;
    const syntaxLower = syntax.toLowerCase();
    if (metaTag.requires_value) {
      if (syntaxLower.startsWith(lower)) {
        result.push({
          commit: `${negPrefix}${syntax}`,
          ns: syntax.slice(0, -1),
          val: '',
          hint: metaTag.hint || 'query',
          partial: true
        });
      }
      continue;
    }
    if (syntaxLower.startsWith(lower)) {
      const commit = `${negPrefix}${syntax}`;
      if (!existing.has(commit)) {
        result.push({ commit, ns: '', val: syntax, hint: metaTag.hint || 'query' });
      }
    }
  }

  if ('type:'.startsWith(lower)) {
    const commit = `${negPrefix}type:`;
    result.push({ commit, ns: 'type', val: '', hint: 'media type', partial: true });
  }
  return result;
}
