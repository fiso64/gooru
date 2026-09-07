export type ViewerConfiguredFitMode = 'fit_window' | 'fit_down_only' | 'original_size_if_fit' | 'actual';
export type ViewerFitMode = ViewerConfiguredFitMode;

export type ViewerGeometryInput = {
  intrinsicWidth: number;
  intrinsicHeight: number;
  viewportWidth: number;
  viewportHeight: number;
  rotation: number;
  fitMode: ViewerFitMode;
  boundActualSizeToFit?: boolean;
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
  // Keep the visual angle continuous so CSS transitions always take the requested
  // quarter-turn direction across 0/360. Geometry normalizes separately for sizing.
  return rotation + (direction === 'left' ? -90 : 90);
}

export function viewerGeometry(input: ViewerGeometryInput): ViewerGeometry {
  const intrinsicWidth = Math.max(0, input.intrinsicWidth);
  const intrinsicHeight = Math.max(0, input.intrinsicHeight);
  const viewportWidth = Math.max(0, input.viewportWidth - 2 * Math.max(0, input.inset ?? 0));
  const viewportHeight = Math.max(0, input.viewportHeight - 2 * Math.max(0, input.inset ?? 0));
  const normalizedRotation = normalizeViewerRotation(input.rotation);

  if (!intrinsicWidth || !intrinsicHeight || !viewportWidth || !viewportHeight) {
    return { width: intrinsicWidth, height: intrinsicHeight, rotation: input.rotation, scale: 1 };
  }

  const quarterTurn = normalizedRotation === 90 || normalizedRotation === 270;
  const rotatedWidth = quarterTurn ? intrinsicHeight : intrinsicWidth;
  const rotatedHeight = quarterTurn ? intrinsicWidth : intrinsicHeight;
  const maxScale = input.maxScale ?? Number.POSITIVE_INFINITY;
  const fitScale = Math.min(viewportWidth / rotatedWidth, viewportHeight / rotatedHeight, maxScale);

  if (input.fitMode === 'actual') {
    const scale = (input.boundActualSizeToFit ?? true) ? Math.min(1, fitScale) : 1;
    return { width: intrinsicWidth * scale, height: intrinsicHeight * scale, rotation: input.rotation, scale };
  }
  // Both non-upscaling policies are intentionally distinct public modes even though
  // contain geometry makes them equivalent today: if the original fits, keep 1:1;
  // otherwise scale down to fit. Keeping both names preserves the requested contract
  // and leaves room for future fit policies without a config migration.
  const scale = input.fitMode === 'fit_window' ? fitScale : Math.min(1, fitScale);

  return {
    width: intrinsicWidth * scale,
    height: intrinsicHeight * scale,
    rotation: input.rotation,
    scale
  };
}

export type ViewerMediaTransform = {
  zoom?: number;
  panX?: number;
  panY?: number;
};

export function viewerMediaStyle(geometry: ViewerGeometry, transform: ViewerMediaTransform = {}): string {
  const zoom = Math.max(0, transform.zoom ?? 1);
  const panX = Number.isFinite(transform.panX) ? (transform.panX ?? 0) : 0;
  const panY = Number.isFinite(transform.panY) ? (transform.panY ?? 0) : 0;
  return [
    'position:absolute',
    'left:50%',
    'top:50%',
    `width:${geometry.width}px`,
    `height:${geometry.height}px`,
    'max-width:none',
    'max-height:none',
    `transform:translate(calc(-50% + ${panX}px), calc(-50% + ${panY}px)) rotate(${geometry.rotation}deg) scale(${zoom})`,
    'transform-origin:center center'
  ].join(';');
}
