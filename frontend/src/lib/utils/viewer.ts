export type ViewerFitMode = 'screen' | 'actual';

export type ViewerGeometryInput = {
  intrinsicWidth: number;
  intrinsicHeight: number;
  viewportWidth: number;
  viewportHeight: number;
  rotation: number;
  fitMode: ViewerFitMode;
  inset?: number;
  maxScale?: number;
};

export type ViewerGeometry = {
  width: number;
  height: number;
  rotation: number;
  scale: number;
};

export function normalizeViewerRotation(rotation: number): number {
  const normalized = ((rotation % 360) + 360) % 360;
  return normalized === 360 ? 0 : normalized;
}

export function rotateViewer(rotation: number, direction: 'left' | 'right'): number {
  return normalizeViewerRotation(rotation + (direction === 'left' ? -90 : 90));
}

export function viewerGeometry(input: ViewerGeometryInput): ViewerGeometry {
  const intrinsicWidth = Math.max(0, input.intrinsicWidth);
  const intrinsicHeight = Math.max(0, input.intrinsicHeight);
  const viewportWidth = Math.max(0, input.viewportWidth - 2 * Math.max(0, input.inset ?? 0));
  const viewportHeight = Math.max(0, input.viewportHeight - 2 * Math.max(0, input.inset ?? 0));
  const rotation = normalizeViewerRotation(input.rotation);

  if (!intrinsicWidth || !intrinsicHeight || !viewportWidth || !viewportHeight) {
    return { width: intrinsicWidth, height: intrinsicHeight, rotation, scale: 1 };
  }

  if (input.fitMode === 'actual') {
    return { width: intrinsicWidth, height: intrinsicHeight, rotation, scale: 1 };
  }

  const quarterTurn = rotation === 90 || rotation === 270;
  const rotatedWidth = quarterTurn ? intrinsicHeight : intrinsicWidth;
  const rotatedHeight = quarterTurn ? intrinsicWidth : intrinsicHeight;
  const maxScale = input.maxScale ?? Number.POSITIVE_INFINITY;
  const scale = Math.min(viewportWidth / rotatedWidth, viewportHeight / rotatedHeight, maxScale);

  return {
    width: intrinsicWidth * scale,
    height: intrinsicHeight * scale,
    rotation,
    scale
  };
}

export function viewerMediaStyle(geometry: ViewerGeometry): string {
  return [
    'position:absolute',
    'left:50%',
    'top:50%',
    `width:${geometry.width}px`,
    `height:${geometry.height}px`,
    'max-width:none',
    'max-height:none',
    `transform:translate(-50%, -50%) rotate(${geometry.rotation}deg)`,
    'transform-origin:center center'
  ].join(';');
}
