export function hasBlockingModal(): boolean {
  return typeof document !== 'undefined' && Boolean(document.querySelector('[role="dialog"][aria-modal="true"]:not(.lightbox)'));
}
