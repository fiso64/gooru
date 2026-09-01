import { describe, expect, it, vi } from 'vitest';
import { claimFocus, type FocusTarget } from './focus';

function target(connected = true) {
  return { focus: vi.fn(), isConnected: connected } satisfies FocusTarget;
}

describe('claimFocus', () => {
  it('moves focus into an overlay and restores the previous control on cleanup', () => {
    const previous = target();
    const overlay = target();

    const restore = claimFocus(overlay, previous);
    expect(overlay.focus).toHaveBeenCalledWith({ preventScroll: true });

    restore();
    expect(previous.focus).toHaveBeenCalledWith({ preventScroll: true });
  });

  it('does not restore focus to a control that left the document', () => {
    const previous = target(false);
    const overlay = target();

    claimFocus(overlay, previous)();
    expect(previous.focus).not.toHaveBeenCalled();
  });
});
