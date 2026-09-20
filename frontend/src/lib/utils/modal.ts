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

  // Allow dialog editing and button activation, but prevent window-level
  // shortcuts from reaching the underlying viewer or library.
  function interceptBackgroundKeys(event: KeyboardEvent) {
    if (event.key === 'Escape' || event.key === 'Tab' || event.key === 'Enter') return;
    const target = event.target;
    if (target instanceof Element && node.contains(target)) {
      if (target.closest('input, textarea, select, [contenteditable], [role="textbox"]')) return;
      if ((event.code === 'Space' || event.key === ' ') && target.closest('button, a, [role="button"]')) return;
    }
    event.stopPropagation();
  }

  document.addEventListener('keydown', interceptBackgroundKeys, true);
  document.addEventListener('fullscreenchange', sync);
  sync();

  return {
    destroy() {
      document.removeEventListener('keydown', interceptBackgroundKeys, true);
      document.removeEventListener('fullscreenchange', sync);
      if (anchor.parentNode) anchor.parentNode.insertBefore(node, anchor);
      anchor.remove();
    }
  };
}
