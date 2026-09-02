import { describe, expect, it } from 'vitest';
import { isPlainTag, plainTagInputContext, plainTagSuggestions, plainTagsFromInput, replacePlainTagInputDraft } from './tagSuggestions';

const tags = [
  { name: 'artist:alice', count: 8 },
  { name: 'artist:bob', count: 4 },
  { name: 'subject:portrait', count: 12 },
  { name: 'landscape', count: 3 },
  { name: '@rating:5', count: 99 }
];

describe('plainTagSuggestions', () => {
  it('matches namespace values and excludes existing tags', () => {
    expect(plainTagSuggestions('artist:a', tags, ['artist:bob']).map((item) => item.name)).toEqual(['artist:alice']);
  });

  it('offers namespace prefixes separately from concrete tags', () => {
    const suggestions = plainTagSuggestions('art', tags);
    expect(suggestions[0]).toEqual({ name: 'artist:', count: 8, kind: 'namespace' });
    expect(suggestions.some((item) => item.name === 'artist:alice' && item.kind === 'tag')).toBe(true);
  });

  it('accepts namespace-only API candidates without treating them as tags', () => {
    const suggestions = plainTagSuggestions('mov', [{ namespace: 'movie', count: 7 }]);
    expect(suggestions).toEqual([{ name: 'movie:', count: 7, kind: 'namespace' }]);
  });

  it('matches valueless and namespaced tag values without exposing metatags', () => {
    expect(plainTagSuggestions('por', tags).map((item) => item.name)).toContain('subject:portrait');
    expect(plainTagSuggestions('@rat', tags)).toEqual([]);
    expect(plainTagSuggestions('-artist', tags)).toEqual([]);
  });
});

describe('isPlainTag', () => {
  it('accepts ordinary tags and rejects search-only or namespace-prefix syntax', () => {
    expect(isPlainTag('artist:alice')).toBe(true);
    expect(isPlainTag('landscape')).toBe(true);
    expect(isPlainTag('artist:')).toBe(false);
    expect(isPlainTag('@rating:5')).toBe(false);
    expect(isPlainTag('-landscape')).toBe(false);
    expect(isPlainTag('two tags')).toBe(false);
  });
});

describe('plainTagsFromInput', () => {
  it('preserves multi-tag entry while excluding search-only syntax and namespace prefixes', () => {
    expect(plainTagsFromInput('artist:alice landscape artist: @rating:5 -exclude landscape')).toEqual(['artist:alice', 'landscape']);
  });
});

describe('plainTagInputContext', () => {
  it('keeps completed tags separate from the active completion fragment', () => {
    expect(plainTagInputContext('landscape artist:a')).toEqual({ committed: ['landscape'], draft: 'artist:a' });
    expect(replacePlainTagInputDraft('landscape artist:a', 'artist:alice')).toBe('landscape artist:alice');
  });

  it('does not offer a completion fragment after trailing whitespace', () => {
    expect(plainTagInputContext('landscape artist:alice ')).toEqual({
      committed: ['landscape', 'artist:alice'],
      draft: ''
    });
  });
});
