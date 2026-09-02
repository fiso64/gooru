import { describe, expect, it } from 'vitest';
import { specialSearchSuggestions } from './searchSuggestions';

describe('specialSearchSuggestions', () => {
  it('suggests @tagged and the type namespace as search-only expressions', () => {
    expect(specialSearchSuggestions('@ta')).toEqual([
      { commit: '@tagged', ns: '', val: '@tagged', hint: 'expression' }
    ]);
    expect(specialSearchSuggestions('ty')).toEqual([
      { commit: 'type:', ns: 'type', val: '', hint: 'media type', partial: true }
    ]);
  });

  it('offers concrete media types after the type prefix', () => {
    expect(specialSearchSuggestions('type:v')).toEqual([
      { commit: 'type:video', ns: 'type', val: 'video', hint: 'media type' }
    ]);
    expect(specialSearchSuggestions('type:').map((item) => item.commit)).toEqual([
      'type:photo',
      'type:video',
      'type:gif',
      'type:audio',
      'type:other'
    ]);
  });

  it('preserves exclusions and suppresses already committed expressions', () => {
    expect(specialSearchSuggestions('-type:g')).toEqual([
      { commit: '-type:gif', ns: 'type', val: 'gif', hint: 'media type' }
    ]);
    expect(specialSearchSuggestions('@tag', ['@tagged'])).toEqual([]);
  });
});
