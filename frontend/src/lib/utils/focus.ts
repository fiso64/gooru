export type FocusTarget = {
  focus: (options?: { preventScroll?: boolean }) => void;
  isConnected?: boolean;
};

export function claimFocus(target: FocusTarget | undefined, previous: FocusTarget | undefined): () => void {
  target?.focus({ preventScroll: true });
  return () => {
    if (previous && previous.isConnected !== false) previous.focus({ preventScroll: true });
  };
}
