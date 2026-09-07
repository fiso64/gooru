import { afterEach, describe, expect, it, vi } from 'vitest';

class FakeIntersectionObserver {
  static instances: FakeIntersectionObserver[] = [];
  callback: IntersectionObserverCallback;
  observed = new Set<Element>();
  root: Element | Document | null;
  rootMargin: string;
  readonly thresholds = [0];

  constructor(callback: IntersectionObserverCallback, options?: IntersectionObserverInit) {
    this.callback = callback;
    this.root = options?.root ?? null;
    this.rootMargin = options?.rootMargin ?? '';
    FakeIntersectionObserver.instances.push(this);
  }

  observe(target: Element) { this.observed.add(target); }
  unobserve(target: Element) { this.observed.delete(target); }
  disconnect() { this.observed.clear(); }
  takeRecords() { return []; }

  intersect(target: Element) {
    this.callback([{ target, isIntersecting: true } as IntersectionObserverEntry], this as unknown as IntersectionObserver);
  }
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.resetModules();
  FakeIntersectionObserver.instances = [];
});

describe('activateNearViewport', () => {
  it('shares one observer for cards in the same scroll pane and activates once', async () => {
    vi.stubGlobal('IntersectionObserver', FakeIntersectionObserver);
    const { activateNearViewport } = await import('./viewportActivation');
    const root = {} as Element;
    const first = {} as Element;
    const second = {} as Element;
    const firstActivation = vi.fn();
    const secondActivation = vi.fn();

    const stopFirst = activateNearViewport(first, root, firstActivation);
    const stopSecond = activateNearViewport(second, root, secondActivation);

    expect(FakeIntersectionObserver.instances).toHaveLength(1);
    const observer = FakeIntersectionObserver.instances[0];
    expect(observer.root).toBe(root);
    expect(observer.rootMargin).toBe('96px 0px');
    expect(observer.observed).toEqual(new Set([first, second]));

    observer.intersect(first);
    expect(firstActivation).toHaveBeenCalledOnce();
    expect(secondActivation).not.toHaveBeenCalled();
    expect(observer.observed.has(first)).toBe(false);

    stopFirst();
    stopSecond();
    expect(observer.observed.size).toBe(0);
  });

  it('keeps separate observers for separate panes', async () => {
    vi.stubGlobal('IntersectionObserver', FakeIntersectionObserver);
    const { activateNearViewport } = await import('./viewportActivation');
    activateNearViewport({} as Element, {} as Element, vi.fn());
    activateNearViewport({} as Element, {} as Element, vi.fn());
    expect(FakeIntersectionObserver.instances).toHaveLength(2);
  });

  it('activates immediately when IntersectionObserver is unavailable', async () => {
    vi.stubGlobal('IntersectionObserver', undefined);
    const { activateNearViewport } = await import('./viewportActivation');
    const activate = vi.fn();
    activateNearViewport({} as Element, {} as Element, activate);
    expect(activate).toHaveBeenCalledOnce();
  });
});
