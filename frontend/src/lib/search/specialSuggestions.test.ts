import { describe, expect, it } from 'vitest';
import { specialSearchSuggestions } from './specialSuggestions';

describe('specialSearchSuggestions', () => {
  it('offers supported reserved expressions without showing them on unrelated input', () => {
    expect(specialSearchSuggestions('@t')).toEqual([
      { commit: '@tagged', ns: '', val: '@tagged', hint: 'has tags' }
    ]);
    expect(specialSearchSuggestions('ty')).toEqual([
      { commit: 'type:', ns: 'type', val: '', hint: 'media type', partial: true }
    ]);
    expect(specialSearchSuggestions('artist')).toEqual([]);
  });

  it('completes supported media kinds after type:', () => {
    expect(specialSearchSuggestions('type:v')).toEqual([
      { commit: 'type:video', ns: 'type', val: 'video', hint: 'media type' }
    ]);
    expect(specialSearchSuggestions('type:')).toHaveLength(5);
  });

  it('supports negation and excludes already committed exact expressions', () => {
    expect(specialSearchSuggestions('-@t')).toEqual([
      { commit: '-@tagged', ns: '', val: '@tagged', hint: 'has tags' }
    ]);
    expect(specialSearchSuggestions('@t', ['@tagged'])).toEqual([]);
    expect(specialSearchSuggestions('type:v', ['type:video'])).toEqual([]);
  });
});
