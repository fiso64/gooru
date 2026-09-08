import { describe, expect, it } from 'vitest';
import { specialSearchSuggestions } from './specialSuggestions';

const querySyntax = [
  { syntax: '@tagged', hint: 'has tags', requires_value: false },
  { syntax: '@filename_contains:', hint: 'filename contains', requires_value: true },
  { syntax: 'type:', hint: 'media type', requires_value: true },
  { syntax: 'type:photo', hint: 'media type', requires_value: false },
  { syntax: 'type:video', hint: 'media type', requires_value: false },
  { syntax: 'ext:', hint: 'file extension', requires_value: true },
  { syntax: 'ext:jpg', hint: 'file extension', requires_value: false },
  { syntax: 'ext:jpeg', hint: 'file extension', requires_value: false },
  { syntax: 'ext:mp4', hint: 'file extension', requires_value: false }
];

describe('specialSearchSuggestions', () => {
  it('uses the backend catalog for predicates and hides them on unrelated input', () => {
    expect(specialSearchSuggestions('@t', [], querySyntax)).toEqual([
      { commit: '@tagged', ns: '', val: '@tagged', hint: 'has tags' }
    ]);
    expect(specialSearchSuggestions('@f', [], querySyntax)).toEqual([
      { commit: '@filename_contains:', ns: '@filename_contains', val: '', hint: 'filename contains', partial: true }
    ]);
    expect(specialSearchSuggestions('artist', [], querySyntax)).toEqual([]);
  });

  it('does not invent reserved syntax when the backend catalog is empty', () => {
    expect(specialSearchSuggestions('@t')).toEqual([]);
    expect(specialSearchSuggestions('type:v')).toEqual([]);
    expect(specialSearchSuggestions('ext:j')).toEqual([]);
  });

  it('completes backend-owned reserved field values', () => {
    expect(specialSearchSuggestions('type:v', [], querySyntax)).toEqual([
      { commit: 'type:video', ns: 'type', val: 'video', hint: 'media type' }
    ]);
    expect(specialSearchSuggestions('ext:j', [], querySyntax)).toEqual([
      { commit: 'ext:jpg', ns: 'ext', val: 'jpg', hint: 'file extension' },
      { commit: 'ext:jpeg', ns: 'ext', val: 'jpeg', hint: 'file extension' }
    ]);
  });

  it('offers field prefixes and supports negation and exact-token exclusion', () => {
    expect(specialSearchSuggestions('ty', [], querySyntax)[0]).toEqual({
      commit: 'type:', ns: 'type', val: '', hint: 'media type', partial: true
    });
    expect(specialSearchSuggestions('-@t', [], querySyntax)).toEqual([
      { commit: '-@tagged', ns: '', val: '@tagged', hint: 'has tags' }
    ]);
    expect(specialSearchSuggestions('type:v', ['type:video'], querySyntax)).toEqual([]);
  });
});
