import { describe, expect, it } from 'vitest';
import { moveCompletionIndex } from './completionNavigation';

describe('completion navigation', () => {
  it('wraps at both ends', () => {
    expect(moveCompletionIndex(2, 3, 1)).toBe(0);
    expect(moveCompletionIndex(0, 3, -1)).toBe(2);
  });
  it('handles singleton and empty panels', () => {
    expect(moveCompletionIndex(0, 1, 1)).toBe(0);
    expect(moveCompletionIndex(0, 0, -1)).toBe(0);
  });
});
