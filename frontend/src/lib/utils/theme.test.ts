import { describe, expect, it } from 'vitest';
import { accentTheme } from './theme';

describe('accentTheme', () => {
  it('normalizes configured hex colors and uses dark ink on bright accents', () => {
    expect(accentTheme('#F4C542')).toEqual({ accent: '#f4c542', accentInk: '#241f12' });
  });

  it('uses light ink on dark accents', () => {
    expect(accentTheme('#3156a8')).toEqual({ accent: '#3156a8', accentInk: '#fffdf6' });
  });

  it('rejects values outside the server-supported color grammar', () => {
    expect(accentTheme('red')).toBeNull();
    expect(accentTheme('#fff')).toBeNull();
  });
});
