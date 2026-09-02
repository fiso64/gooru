export type ViewerFitMode = 'screen' | 'actual';

export type ViewerSize = {
  width: number;
  height: number;
};

export function normalizeViewerRotation(rotation: number): number {
  const normalized = rotation % 360;
  return normalized < 0 ? normalized + 360 : normalized;
}

export function rotateViewerLeft(rotation: number): number {
  return normalizeViewerRotation(rotation - 90);
}

export function rotateViewerRight(rotation: number): number {
  return normalizeViewerRotation(rotation + 90);
}

export function rotatedViewerSize(size: ViewerSize, rotation: number): ViewerSize {
  const normalized = normalizeViewerRotation(rotation);
  if (normalized === 90 || normalized === 270) {
    return { width: size.height, height: size.width };
  }
  return size;
}

export function viewerScale(
  intrinsic: ViewerSize,
  available: ViewerSize,
  rotation: number,
  fitMode: ViewerFitMode
): number {
  if (fitMode === 'actual') return 1;
  if (intrinsic.width <= 0 || intrinsic.height <= 0 || available.width <= 0 || available.height <= 0) return 1;

  const rotated = rotatedViewerSize(intrinsic, rotation);
  return Math.min(available.width / rotated.width, available.height / rotated.height);
}

export function viewerTransform(rotation: number, scale: number): string {
  return `translate(-50%, -50%) rotate(${normalizeViewerRotation(rotation)}deg) scale(${scale})`;
}
