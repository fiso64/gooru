import { describe, expect, it } from 'vitest';
import { offsetPageToken, paginationWindow } from './pagination';

describe('paginationWindow', () => {
  it('keeps first, last, current, nearby pages, and gap markers', () => {
    expect(paginationWindow(6, 12)).toEqual([1, 'ellipsis', 4, 5, 6, 7, 8, 'ellipsis', 12]);
  });

  it('does not add gap markers for contiguous page ranges', () => {
    expect(paginationWindow(2, 5)).toEqual([1, 2, 3, 4, 5]);
    expect(paginationWindow(1, 3)).toEqual([1, 2, 3]);
  });
});

describe('offsetPageToken', () => {
  it('encodes a direct page offset in the server page-token format', () => {
    expect(atob(offsetPageToken(9, 25))).toBe('offset:225');
    expect(offsetPageToken(0, 25)).toBe('');
  });
});
