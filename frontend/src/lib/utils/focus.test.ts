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

  it('drops a focus-visible state created only while restoring a pointer opener', () => {
    let visible = false;
    const previous = {
      focus: vi.fn(() => { visible = true; }),
      blur: vi.fn(),
      matches: vi.fn((selector: string) => selector === ':focus-visible' && visible),
      isConnected: true
    } satisfies FocusTarget;
    const overlay = target();

    claimFocus(overlay, previous)();

    expect(previous.focus).toHaveBeenCalledWith({ preventScroll: true });
    expect(previous.blur).toHaveBeenCalledOnce();
  });

  it('preserves focus restoration for an opener that already had keyboard-visible focus', () => {
    const previous = {
      focus: vi.fn(),
      blur: vi.fn(),
      matches: vi.fn((selector: string) => selector === ':focus-visible'),
      isConnected: true
    } satisfies FocusTarget;
    const overlay = target();

    claimFocus(overlay, previous)();

    expect(previous.focus).toHaveBeenCalledWith({ preventScroll: true });
    expect(previous.blur).not.toHaveBeenCalled();
  });
});
