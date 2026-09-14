export type FocusTarget = {
  focus: (options?: { preventScroll?: boolean }) => void;
  blur?: () => void;
  matches?: (selector: string) => boolean;
  isConnected?: boolean;
};

export function claimFocus(target: FocusTarget | undefined, previous: FocusTarget | undefined): () => void {
  const previousHadVisibleFocus = previous?.matches?.(':focus-visible');
  target?.focus({ preventScroll: true });
  return () => {
    if (!previous || previous.isConnected === false) return;
    previous.focus({ preventScroll: true });
    if (previousHadVisibleFocus === false && previous.matches?.(':focus-visible')) previous.blur?.();
  };
}
