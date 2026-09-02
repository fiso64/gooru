import { describe, expect, it } from 'vitest';
import {
  normalizeViewerRotation,
  rotateViewerLeft,
  rotateViewerRight,
  rotatedViewerSize,
  viewerScale,
  viewerTransform
} from './viewerGeometry';

describe('viewer geometry', () => {
  it('normalizes and steps quarter-turn rotation', () => {
    expect(normalizeViewerRotation(-90)).toBe(270);
    expect(rotateViewerLeft(0)).toBe(270);
    expect(rotateViewerRight(270)).toBe(0);
  });

  it('swaps occupied dimensions for quarter turns', () => {
    expect(rotatedViewerSize({ width: 1600, height: 900 }, 90)).toEqual({ width: 900, height: 1600 });
    expect(rotatedViewerSize({ width: 1600, height: 900 }, 180)).toEqual({ width: 1600, height: 900 });
  });

  it('fits rotated media against the correct viewport axes', () => {
    expect(viewerScale({ width: 1600, height: 900 }, { width: 1000, height: 700 }, 0, 'screen')).toBeCloseTo(0.625);
    expect(viewerScale({ width: 1600, height: 900 }, { width: 1000, height: 700 }, 90, 'screen')).toBeCloseTo(0.4375);
  });

  it('keeps one-to-one mode at native scale', () => {
    expect(viewerScale({ width: 8000, height: 4000 }, { width: 1000, height: 700 }, 90, 'actual')).toBe(1);
  });

  it('builds a centered transform with normalized rotation', () => {
    expect(viewerTransform(-90, 0.5)).toBe('translate(-50%, -50%) rotate(270deg) scale(0.5)');
  });
});
