import { describe, expect, it } from 'vitest';
import { normalizeViewerRotation, rotateViewer, viewerGeometry, viewerMediaStyle } from './viewer';

describe('viewer geometry policy', () => {
  it('normalizes and rotates in quarter turns', () => {
    expect(normalizeViewerRotation(-90)).toBe(270);
    expect(rotateViewer(0, 'left')).toBe(270);
    expect(rotateViewer(270, 'right')).toBe(0);
  });

  it('fits unrotated media maximally inside the usable viewport', () => {
    const geometry = viewerGeometry({
      intrinsicWidth: 1600,
      intrinsicHeight: 900,
      viewportWidth: 1000,
      viewportHeight: 800,
      rotation: 0,
      fitMode: 'screen',
      inset: 20
    });
    expect(geometry.width).toBeCloseTo(960);
    expect(geometry.height).toBeCloseTo(540);
  });

  it('accounts for swapped bounds when media is rotated', () => {
    const geometry = viewerGeometry({
      intrinsicWidth: 1600,
      intrinsicHeight: 900,
      viewportWidth: 1000,
      viewportHeight: 800,
      rotation: 90,
      fitMode: 'screen'
    });
    expect(geometry.width).toBeCloseTo(800);
    expect(geometry.height).toBeCloseTo(450);
    expect(geometry.rotation).toBe(90);
  });

  it('keeps one-to-one mode at intrinsic dimensions regardless of rotation', () => {
    const geometry = viewerGeometry({
      intrinsicWidth: 640,
      intrinsicHeight: 480,
      viewportWidth: 200,
      viewportHeight: 200,
      rotation: 270,
      fitMode: 'actual'
    });
    expect(geometry).toEqual({ width: 640, height: 480, rotation: 270, scale: 1 });
  });

  it('can cap fit-to-screen scaling for media that should stay at native size', () => {
    const geometry = viewerGeometry({
      intrinsicWidth: 320,
      intrinsicHeight: 200,
      viewportWidth: 1920,
      viewportHeight: 1080,
      rotation: 90,
      fitMode: 'screen',
      maxScale: 1
    });
    expect(geometry).toEqual({ width: 320, height: 200, rotation: 90, scale: 1 });
  });

  it('renders geometry as a centered rotation transform', () => {
    expect(viewerMediaStyle({ width: 100, height: 50, rotation: 90, scale: 1 })).toContain('rotate(90deg)');
  });
});
