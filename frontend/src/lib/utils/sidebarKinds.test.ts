import { describe, expect, it } from 'vitest';
import { appendSidebarKind, queryWithoutSidebarKind, replaceSidebarKind, sidebarKindActive } from './sidebarKinds';

describe('sidebar kind query helpers', () => {
  it('replaces only sidebar kind terms while preserving unrelated query terms', () => {
    expect(replaceSidebarKind('technology type:video rating:safe', 'type:photo')).toBe('technology rating:safe type:photo');
    expect(replaceSidebarKind('technology ext:cbz', 'type:gif')).toBe('technology type:gif');
  });

  it('toggles the active kind off and normalizes legacy counts queries', () => {
    expect(replaceSidebarKind('technology type:photo', 'type:photo')).toBe('technology');
    expect(queryWithoutSidebarKind('technology TYPE:VIDEO ext:cbz rating:safe')).toBe('technology rating:safe');
    expect(sidebarKindActive('technology TYPE:VIDEO', 'type:video')).toBe(true);
    expect(appendSidebarKind('technology type:photo', 'ext:cbz')).toBe('technology ext:cbz');
  });
});
