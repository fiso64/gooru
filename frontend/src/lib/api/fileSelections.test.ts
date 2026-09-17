import { afterEach, describe, expect, it } from 'vitest';
import { createFileSelection } from './fileSelections';

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
});

describe('createFileSelection', () => {
  it.each([
    ['empty query', '', ''],
    ['whitespace-only query', '  \n ', ''],
    ['non-empty query', '  rating:safe  ', 'rating:safe']
  ])('serializes %s without a wildcard fallback', async (_name, query, expectedQuery) => {
    let requestBody = '';
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      expect(input).toBe('/api/v1/file-selections');
      requestBody = String(init?.body ?? '');
      return Response.json({ id: 'selection-one', count: 1 });
    }) as typeof fetch;

    await createFileSelection('csrf-token', query);

    expect(JSON.parse(requestBody)).toEqual({ query: expectedQuery });
  });
});
