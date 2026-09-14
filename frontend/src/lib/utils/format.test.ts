import { describe, expect, it } from 'vitest';
import { groupTags } from './format';

describe('groupTags', () => {
  it('keeps namespaced groups ahead of non-namespaced tags regardless of insertion order', () => {
    expect(groupTags(['key1:value1', 'tag', 'key2:value2'])).toEqual([
      { namespace: 'key1', tags: ['key1:value1'] },
      { namespace: 'key2', tags: ['key2:value2'] },
      { namespace: '', tags: ['tag'] }
    ]);
  });

  it('keeps a single non-namespaced group when no namespaces exist', () => {
    expect(groupTags(['one', 'two'])).toEqual([
      { namespace: '', tags: ['one', 'two'] }
    ]);
  });
});
