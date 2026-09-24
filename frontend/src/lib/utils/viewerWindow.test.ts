import { describe, expect, it } from 'vitest';
import { advanceViewerWindow, windowNeighbor, type ViewerWindow } from './viewerWindow';

const files = ['a','b','c','d','e','f','g','h'].map((id) => ({ id }));

function around(index: number): ViewerWindow<{ id: string }> {
  const file = (offset: number) => files[(index + offset + files.length) % files.length];
  return { anchor: file(0).id, context: 'tag:x|added|desc', before: [-1,-2,-3,-4,-5].map(file), after: [1,2,3,4,5].map(file) };
}

describe('viewer navigation metadata window', () => {
  it('wraps around the complete sequence even when the grid page has a single file', () => {
    let window = around(0);
    expect(windowNeighbor(window, -1)?.id).toBe('h');
    window = advanceViewerWindow(window, files[0], -1);
    expect(window.anchor).toBe('h');
    expect(windowNeighbor(window, 1)?.id).toBe('a');
  });

  it('supports consecutive steps in either direction without another request', () => {
    let window = around(3);
    window = advanceViewerWindow(window, files[3], 1);
    window = advanceViewerWindow(window, files[4], 1);
    expect(window.anchor).toBe('f');
    expect(windowNeighbor(window, -1)?.id).toBe('e');
    window = advanceViewerWindow(window, files[5], -1);
    expect(window.anchor).toBe('e');
    expect(windowNeighbor(window, 1)?.id).toBe('f');
  });

  it('keeps the anchor out of the cached neighbors for a two-item listing', () => {
    const a = { id: 'a' }, b = { id: 'b' };
    let window: ViewerWindow<{ id: string }> = { anchor: 'a', context: '', complete: true, before: [b], after: [b] };
    window = advanceViewerWindow(window, a, 1);
    expect(window.anchor).toBe('b');
    expect(window.before.map((file) => file.id)).toEqual(['a']);
    expect(window.after.map((file) => file.id)).toEqual(['a']);
    window = advanceViewerWindow(window, b, 1);
    expect(window.anchor).toBe('a');
    expect(windowNeighbor(window, -1)?.id).toBe('b');
  });

  it('rotates a complete short filtered listing without exhausting either side', () => {
    const small = [{id:'a'}, {id:'b'}, {id:'c'}];
    let window: ViewerWindow<{id:string}> = {
      anchor: 'a', context: '', complete: true,
      before: [small[2], small[1]], after: [small[1], small[2]]
    };
    for (let step = 0; step < 20; step++) {
      const current = small[step % 3];
      expect(window.anchor).toBe(current.id);
      expect(windowNeighbor(window, 1)?.id).toBe(small[(step+1)%3].id);
      expect(windowNeighbor(window, -1)?.id).toBe(small[(step+2)%3].id);
      window = advanceViewerWindow(window, current, 1);
    }
  });

  it('does not invent neighbors for a singleton or exhausted buffer', () => {
    const window: ViewerWindow<{id:string}> = { anchor: 'a', context: '', before: [], after: [] };
    expect(windowNeighbor(window, 1)).toBeUndefined();
    expect(advanceViewerWindow(window, files[0], -1)).toBe(window);
  });
});
