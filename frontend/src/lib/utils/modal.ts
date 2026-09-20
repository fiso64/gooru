/** A viewer is itself a dialog; only an additional dialog suspends its shortcuts. */
export function hasBlockingModal(): boolean {
  return typeof document !== 'undefined'
    && Boolean(document.querySelector('[role="dialog"][aria-modal="true"]:not(.lightbox)'));
}

/**
 * The Fullscreen API paints only the fullscreen element and its descendants.
 * Move active overlays into a fullscreen viewer stage, retaining their normal
 * DOM position when fullscreen ends or the owning component unmounts.
 */
export function placeModalInFullscreenViewer(node: HTMLElement) {
  const anchor = document.createComment('modal home');
  node.before(anchor);

  function sync() {
    const fullscreen = document.fullscreenElement;
    if (fullscreen?.classList.contains('viewer-stage')) {
      if (node.parentNode !== fullscreen) fullscreen.appendChild(node);
    } else if (anchor.parentNode && node.parentNode !== anchor.parentNode) {
      anchor.parentNode.insertBefore(node, anchor);
    }
  }

  document.addEventListener('fullscreenchange', sync);
  sync();

  return {
    destroy() {
      document.removeEventListener('fullscreenchange', sync);
      if (anchor.parentNode) anchor.parentNode.insertBefore(node, anchor);
      anchor.remove();
    }
  };
}
