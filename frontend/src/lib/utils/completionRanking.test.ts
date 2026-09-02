import { describe, expect, it } from 'vitest';
import { compareCompletionRank, completionComponentStartsWith } from './completionRanking';

describe('completion ranking', () => {
  it('matches query prefixes at colon and underscore component boundaries', () => {
    expect(completionComponentStartsWith('series:my_hero_academia', 'hero')).toBe(true);
    expect(completionComponentStartsWith('series:superhero', 'hero')).toBe(false);
    expect(completionComponentStartsWith('Artist:Alice', 'ali')).toBe(true);
  });

  it('ranks component-prefix matches before higher-use substring-only matches', () => {
    const items = [
      { name: 'series:superhero', count: 100 },
      { name: 'series:my_hero_academia', count: 3 },
      { name: 'heroic', count: 7 },
      { name: 'subject:hero', count: 5 }
    ];
    expect(items.sort((a, b) => compareCompletionRank(a.name, b.name, 'hero', a.count, b.count)).map((item) => item.name)).toEqual([
      'heroic',
      'subject:hero',
      'series:my_hero_academia',
      'series:superhero'
    ]);
  });

  it('uses count then name for deterministic ties within the same prefix class', () => {
    const items = [
      { name: 'subject:portrait_zebra', count: 2 },
      { name: 'subject:portrait_alpha', count: 2 },
      { name: 'portrait', count: 9 }
    ];
    expect(items.sort((a, b) => compareCompletionRank(a.name, b.name, 'por', a.count, b.count)).map((item) => item.name)).toEqual([
      'portrait',
      'subject:portrait_alpha',
      'subject:portrait_zebra'
    ]);
  });
});
