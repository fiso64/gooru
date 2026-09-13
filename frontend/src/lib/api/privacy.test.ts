import { afterEach, describe, expect, it, vi } from 'vitest';
import { selectProtectedReadTransport, setProtectedReadTransport } from './privacy';

afterEach(() => setProtectedReadTransport(false));

describe('selectProtectedReadTransport', () => {
  it('uses only the clear query transport in ordinary mode', () => {
    const clearQuery = vi.fn(() => 'clear');
    const protectedBody = vi.fn(() => 'protected');

    setProtectedReadTransport(false);
    expect(selectProtectedReadTransport({ clearQuery, protectedBody })).toBe('clear');
    expect(clearQuery).toHaveBeenCalledOnce();
    expect(protectedBody).not.toHaveBeenCalled();
  });

  it('uses only the protected body transport in protected mode', () => {
    const clearQuery = vi.fn(() => 'clear');
    const protectedBody = vi.fn(() => 'protected');

    setProtectedReadTransport(true);
    expect(selectProtectedReadTransport({ clearQuery, protectedBody })).toBe('protected');
    expect(protectedBody).toHaveBeenCalledOnce();
    expect(clearQuery).not.toHaveBeenCalled();
  });
});
