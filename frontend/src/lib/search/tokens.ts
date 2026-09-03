export interface SearchToken {
  ns: string;
  val: string;
  neg: boolean;
}

function unquoteSearchToken(value: string): string {
  if (value.length < 2 || !value.startsWith('"') || !value.endsWith('"')) return value;
  return value.slice(1, -1).replace(/\\([\\"])/g, '$1');
}

function quoteSearchToken(value: string): string {
  if (!/\s/.test(value)) return value;
  return `"${value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
}

export function parseSearchToken(input: string): SearchToken {
  let value = input.trim();
  let neg = false;
  if (value.startsWith('-')) {
    neg = true;
    value = value.slice(1);
  }
  value = unquoteSearchToken(value);
  const colon = value.indexOf(':');
  if (colon === -1) return { ns: '', val: value, neg };
  return { ns: value.slice(0, colon), val: value.slice(colon + 1), neg };
}

export function searchTokenToString(token: SearchToken): string {
  const body = token.ns ? `${token.ns}:${token.val}` : token.val;
  return `${token.neg ? '-' : ''}${quoteSearchToken(body)}`;
}

function splitSearchQuery(query: string): string[] {
  const tokens: string[] = [];
  let current = '';
  let quoted = false;
  let escaped = false;
  for (const char of query.trim()) {
    if (escaped) {
      current += char;
      escaped = false;
      continue;
    }
    if (quoted && char === '\\') {
      current += char;
      escaped = true;
      continue;
    }
    if (char === '"') {
      current += char;
      quoted = !quoted;
      continue;
    }
    if (/\s/.test(char) && !quoted) {
      if (current) tokens.push(current);
      current = '';
      continue;
    }
    current += char;
  }
  if (current) tokens.push(current);
  return tokens;
}

export function parseSearchQuery(query: string): SearchToken[] {
  return splitSearchQuery(query)
    .filter(Boolean)
    .map(parseSearchToken)
    .filter((token) => token.ns || token.val);
}

export function searchTokensToQuery(tokens: SearchToken[]): string {
  return tokens.map(searchTokenToString).join(' ');
}
