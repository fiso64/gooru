import { describe, expect, it } from 'vitest';
import { isPlainTag, mergeTagCandidateCounts, plainTagSuggestions, plainTagsFromInput } from './tagSuggestions';

const tags = [
  { name: 'artist:alice', count: 8 },
  { name: 'artist:bob', count: 4 },
  { name: 'subject:portrait', count: 12 },
  { name: 'landscape', count: 3 },
  { name: '@rating:5', count: 99 }
];

describe('mergeTagCandidateCounts', () => {
  it('injects staged-only tags and increments backend counts once per staged item', () => {
    expect(mergeTagCandidateCounts(tags, [
      ['artist:alice', 'local:only', 'artist:alice'],
      ['ARTIST:ALICE', 'local:only'],
      ['artist:bob']
    ])).toEqual([
      { name: 'artist:alice', count: 10 },
      { name: 'artist:bob', count: 5 },
      { name: 'subject:portrait', count: 12 },
      { name: 'landscape', count: 3 },
      { name: '@rating:5', count: 99 },
      { name: 'local:only', count: 2 }
    ]);
  });

  it('increments structured backend candidates without adding a duplicate staged candidate', () => {
    expect(mergeTagCandidateCounts(
      [{ namespace: 'artist', value: 'alice', count: 8 }],
      [['artist:alice'], ['artist:alice']]
    )).toEqual([{ namespace: 'artist', value: 'alice', count: 10 }]);
  });
});

describe('plainTagSuggestions', () => {
  it('uses the base unique-file count for a real namespace and never invents namespaces for plain tags', () => {
    const candidates = [
      { name: 'animal', count: 3 },
      { name: 'animal:cat', namespace: 'animal', value: 'cat', count: 1 },
      { name: 'animal:hamster', namespace: 'animal', value: 'hamster', count: 1 },
      { name: 'animal:horse', namespace: 'animal', value: 'horse', count: 1 },
      { name: 'ai', count: 2 },
      { name: 'a', count: 1 }
    ];

    const suggestions = plainTagSuggestions('a', candidates, [], 10);
    expect(suggestions.slice(0, 4)).toEqual([
      { name: 'animal', count: 3, kind: 'tag' },
      { name: 'animal:', count: 3, kind: 'namespace' },
      { name: 'ai', count: 2, kind: 'tag' },
      { name: 'a', count: 1, kind: 'tag' }
    ]);
    expect(suggestions.some(({ name }) => name === 'a:' || name === 'ai:')).toBe(false);
  });

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

  it('ranks component-prefix matches before more-used substring-only matches', () => {
    const candidates = [
      { name: 'series:superhero', count: 100 },
      { name: 'series:my_hero_academia', count: 3 },
      { name: 'heroic', count: 7 },
      { name: 'subject:hero', count: 5 }
    ];
    expect(plainTagSuggestions('hero', candidates).map((item) => item.name)).toEqual([
      'heroic',
      'subject:hero',
      'series:my_hero_academia',
      'series:superhero'
    ]);
  });

  it('does not let a substring-only namespace outrank a prefix tag', () => {
    const candidates = [
      { name: 'character:alice', count: 100 },
      { name: 'technology', count: 2 }
    ];
    const names = plainTagSuggestions('te', candidates).map((item) => item.name);
    expect(names[0]).toBe('technology');
    expect(names).toContain('character:');
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
