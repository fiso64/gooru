import { describe, expect, it } from 'vitest';
import { normalizeViewerRotation, rotateViewer, viewerGeometry, viewerMediaStyle } from './viewer';

describe('viewer geometry policy', () => {
  it('normalizes geometry while preserving continuous visual rotation', () => {
    expect(normalizeViewerRotation(-90)).toBe(270);
    expect(rotateViewer(0, 'left')).toBe(-90);
    expect(rotateViewer(270, 'right')).toBe(360);
    expect(rotateViewer(-270, 'left')).toBe(-360);
  });

  it('fits unrotated media maximally inside the usable viewport', () => {
    const geometry = viewerGeometry({
      intrinsicWidth: 1600,
      intrinsicHeight: 900,
      viewportWidth: 1000,
      viewportHeight: 800,
      rotation: 0,
      fitMode: 'fit_window',
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
      rotation: 450,
      fitMode: 'fit_window'
    });
    expect(geometry.width).toBeCloseTo(800);
    expect(geometry.height).toBeCloseTo(450);
    expect(geometry.rotation).toBe(450);
  });

  it('keeps one-to-one mode at intrinsic dimensions regardless of rotation', () => {
    const geometry = viewerGeometry({
      intrinsicWidth: 640,
      intrinsicHeight: 480,
      viewportWidth: 200,
      viewportHeight: 200,
      rotation: -90,
      fitMode: 'actual',
      boundActualSizeToFit: false
    });
    expect(geometry).toEqual({ width: 640, height: 480, rotation: -90, scale: 1 });
  });

  it('caps actual mode to fit by default and honors rotated bounds', () => {
    const geometry = viewerGeometry({
      intrinsicWidth: 1200,
      intrinsicHeight: 400,
      viewportWidth: 500,
      viewportHeight: 300,
      rotation: 90,
      fitMode: 'actual'
    });
    expect(geometry.scale).toBeCloseTo(0.25);
    expect(geometry.width).toBeCloseTo(300);
    expect(geometry.height).toBeCloseTo(100);
  });

  it('preserves intrinsic actual-mode geometry when the fit cap is disabled', () => {
    const geometry = viewerGeometry({
      intrinsicWidth: 1200,
      intrinsicHeight: 400,
      viewportWidth: 500,
      viewportHeight: 300,
      rotation: 90,
      fitMode: 'actual',
      boundActualSizeToFit: false
    });
    expect(geometry).toEqual({ width: 1200, height: 400, rotation: 90, scale: 1 });
  });

  it('can cap fit-to-screen scaling for media that should stay at native size', () => {
    const geometry = viewerGeometry({
      intrinsicWidth: 320,
      intrinsicHeight: 200,
      viewportWidth: 1920,
      viewportHeight: 1080,
      rotation: 90,
      fitMode: 'fit_window',
      maxScale: 1
    });
    expect(geometry).toEqual({ width: 320, height: 200, rotation: 90, scale: 1 });
  });

  it('does not upscale small media in non-upscaling modes', () => {
    for (const fitMode of ['fit_down_only', 'original_size_if_fit'] as const) {
      const geometry = viewerGeometry({ intrinsicWidth: 320, intrinsicHeight: 200, viewportWidth: 1920, viewportHeight: 1080, rotation: 0, fitMode });
      expect(geometry.scale).toBe(1);
      expect(geometry.width).toBe(320);
      expect(geometry.height).toBe(200);
    }
  });

  it('still scales oversized media down in non-upscaling modes', () => {
    const geometry = viewerGeometry({ intrinsicWidth: 2000, intrinsicHeight: 1000, viewportWidth: 1000, viewportHeight: 700, rotation: 0, fitMode: 'original_size_if_fit' });
    expect(geometry.scale).toBeCloseTo(0.5);
  });

  it('renders geometry as a centered rotation transform', () => {
    expect(viewerMediaStyle({ width: 100, height: 50, rotation: 360, scale: 1 })).toContain('rotate(360deg)');
  });
});
