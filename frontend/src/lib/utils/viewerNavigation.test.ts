import { describe, expect, it } from 'vitest';
import { previewAfterRemoval, previewNeighbor } from './viewerNavigation';

const items = [{ id: 'a' }, { id: 'b' }, { id: 'c' }];

describe('previewNeighbor', () => {
  it('moves forward and backward with wraparound', () => {
    expect(previewNeighbor(items[1], items, 1)?.id).toBe('c');
    expect(previewNeighbor(items[0], items, -1)?.id).toBe('c');
    expect(previewNeighbor(items[2], items, 1)?.id).toBe('a');
  });

  it('prefers the preceding item after removal', () => {
    expect(previewAfterRemoval('b', items)?.id).toBe('a');
    expect(previewAfterRemoval('a', items)?.id).toBe('b');
    expect(previewAfterRemoval('c', items)?.id).toBe('b');
    expect(previewAfterRemoval('a', [items[0]])).toBeNull();
  });

  it('returns null when the active item is outside the navigation set', () => {
    expect(previewNeighbor({ id: 'missing' }, items, 1)).toBeNull();
  });
});
