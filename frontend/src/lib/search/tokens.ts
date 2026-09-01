export interface SearchToken {
  ns: string;
  val: string;
  neg: boolean;
}

export function parseSearchToken(input: string): SearchToken {
  let value = input.trim();
  let neg = false;
  if (value.startsWith('-')) {
    neg = true;
    value = value.slice(1);
  }
  const colon = value.indexOf(':');
  if (colon === -1) return { ns: '', val: value, neg };
  return { ns: value.slice(0, colon), val: value.slice(colon + 1), neg };
}

export function searchTokenToString(token: SearchToken): string {
  const body = token.ns ? `${token.ns}:${token.val}` : token.val;
  return `${token.neg ? '-' : ''}${body}`;
}

export function parseSearchQuery(query: string): SearchToken[] {
  return query
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .map(parseSearchToken)
    .filter((token) => token.ns || token.val);
}

export function searchTokensToQuery(tokens: SearchToken[]): string {
  return tokens.map(searchTokenToString).join(' ');
}
