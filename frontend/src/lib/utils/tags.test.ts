import { describe, expect, it } from 'vitest';
import { applyTagOperation } from './tags';

describe('applyTagOperation', () => {
  it('adds new tags without duplicating existing tags', () => {
    expect(applyTagOperation(['a', 'ns:b'], 'add', ['ns:b', 'c'])).toEqual(['a', 'ns:b', 'c']);
  });

  it('removes only requested tags', () => {
    expect(applyTagOperation(['a', 'b', 'c'], 'remove', ['b', 'missing'])).toEqual(['a', 'c']);
  });

  it('replaces the complete tag set for set operations', () => {
    expect(applyTagOperation(['old'], 'set', ['new', 'new', 'ns:value'])).toEqual(['new', 'ns:value']);
  });
});
