import { describe, expect, it } from 'vitest';
import { isPlainTag, plainTagSuggestions } from './tagSuggestions';

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

  it('matches valueless and namespaced tag values without exposing metatags', () => {
    expect(plainTagSuggestions('por', tags).map((item) => item.name)).toEqual(['subject:portrait']);
    expect(plainTagSuggestions('@rat', tags)).toEqual([]);
    expect(plainTagSuggestions('-artist', tags)).toEqual([]);
  });
});

describe('isPlainTag', () => {
  it('accepts ordinary tags and rejects search-only syntax', () => {
    expect(isPlainTag('artist:alice')).toBe(true);
    expect(isPlainTag('landscape')).toBe(true);
    expect(isPlainTag('@rating:5')).toBe(false);
    expect(isPlainTag('-landscape')).toBe(false);
    expect(isPlainTag('two tags')).toBe(false);
  });
});
