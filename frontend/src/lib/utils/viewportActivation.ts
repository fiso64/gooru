type Activation = () => void;

interface ObserverState {
  observer: IntersectionObserver;
  activations: WeakMap<Element, Activation>;
}

const rootObservers = new WeakMap<Element, ObserverState>();
let viewportObserverState: ObserverState | undefined;

function createObserver(root: Element | null): ObserverState | undefined {
  if (typeof IntersectionObserver === 'undefined') return undefined;
  const activations = new WeakMap<Element, Activation>();
  const state: ObserverState = {
    activations,
    observer: new IntersectionObserver((entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue;
        const activate = activations.get(entry.target);
        if (!activate) continue;
        activations.delete(entry.target);
        state.observer.unobserve(entry.target);
        activate();
      }
    }, { root, rootMargin: '250px 0px' })
  };
  return state;
}

function observerFor(root: Element | null) {
  if (!root) {
    viewportObserverState ??= createObserver(null);
    return viewportObserverState;
  }
  let state = rootObservers.get(root);
  if (!state) {
    state = createObserver(root);
    if (state) rootObservers.set(root, state);
  }
  return state;
}

export function activateNearViewport(node: Element, root: Element | null, activate: Activation) {
  const state = observerFor(root);
  if (!state) {
    activate();
    return () => {};
  }
  state.activations.set(node, activate);
  state.observer.observe(node);
  return () => {
    state.activations.delete(node);
    state.observer.unobserve(node);
  };
}
