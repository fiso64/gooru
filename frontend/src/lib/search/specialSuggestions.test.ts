import { describe, expect, it } from 'vitest';
import { specialSearchSuggestions } from './specialSuggestions';

const metaTags = [
  { syntax: '@tagged', hint: 'has tags', requires_value: false },
  { syntax: '@filename_contains:', hint: 'filename contains', requires_value: true }
];

describe('specialSearchSuggestions', () => {
  it('uses the backend catalog for metatags and hides them on unrelated input', () => {
    expect(specialSearchSuggestions('@t', [], metaTags)).toEqual([
      { commit: '@tagged', ns: '', val: '@tagged', hint: 'has tags' }
    ]);
    expect(specialSearchSuggestions('@f', [], metaTags)).toEqual([
      { commit: '@filename_contains:', ns: '@filename_contains', val: '', hint: 'filename contains', partial: true }
    ]);
    expect(specialSearchSuggestions('artist', [], metaTags)).toEqual([]);
  });

  it('does not invent metatags when the backend catalog is empty', () => {
    expect(specialSearchSuggestions('@t')).toEqual([]);
    expect(specialSearchSuggestions('@f')).toEqual([]);
  });

  it('completes supported media kinds after type:', () => {
    expect(specialSearchSuggestions('type:v', [], metaTags)).toEqual([
      { commit: 'type:video', ns: 'type', val: 'video', hint: 'media type' }
    ]);
    expect(specialSearchSuggestions('type:', [], metaTags)).toHaveLength(5);
  });

  it('supports negation and excludes already committed exact expressions', () => {
    expect(specialSearchSuggestions('-@t', [], metaTags)).toEqual([
      { commit: '-@tagged', ns: '', val: '@tagged', hint: 'has tags' }
    ]);
    expect(specialSearchSuggestions('@t', ['@tagged'], metaTags)).toEqual([]);
    expect(specialSearchSuggestions('type:v', ['type:video'], metaTags)).toEqual([]);
  });
});
