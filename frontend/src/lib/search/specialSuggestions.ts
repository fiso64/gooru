export type SpecialSearchSuggestion = {
  commit: string;
  ns: string;
  val: string;
  hint: string;
  partial?: boolean;
};

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
  const result: SpecialSearchSuggestion[] = [];

  for (const definition of metaTags) {
    const syntax = definition.syntax.trim();
    if (!syntax) continue;
    const syntaxLower = syntax.toLowerCase();
    if (!syntaxLower.startsWith(lower)) continue;

    if (definition.requires_value) {
      result.push({
        commit: `${negPrefix}${syntax}`,
        ns: syntax.endsWith(':') ? syntax.slice(0, -1) : syntax,
        val: '',
        hint: definition.hint || 'query',
        partial: true
      });
      continue;
    }

    const commit = `${negPrefix}${syntax}`;
    if (existing.has(commit)) continue;
    const colon = syntax.indexOf(':');
    result.push({
      commit,
      ns: colon >= 0 && !syntax.startsWith('@') ? syntax.slice(0, colon) : '',
      val: colon >= 0 && !syntax.startsWith('@') ? syntax.slice(colon + 1) : syntax,
      hint: definition.hint || 'query'
    });
  }

  return result;
}
