import type { ViewerFitMode } from '$lib/utils/viewer';
import {
  readBrowserSessionPreference,
  writeBrowserSessionPreference
} from '$lib/utils/browserStorage';

export type ViewerRotation = 0 | 90 | 180 | 270;
export type ViewerScaling = 'smooth' | 'nearest';

export interface ViewerSessionPreferences {
  preferOriginal: boolean;
  rotation: ViewerRotation;
  fitMode: ViewerFitMode;
  scaling: ViewerScaling;
}

type StoredViewerSessionPreferences = {
  version: 1;
  preferOriginal?: boolean;
  rotation?: ViewerRotation;
  fitMode?: ViewerFitMode | 'screen';
  scaling?: ViewerScaling;
};
type SessionStorage = Pick<Storage, 'getItem' | 'setItem'>;

export const viewerSessionStorageKey = 'gooru.viewer.preferences.v1' as const;

function isStoredViewerSessionPreferences(value: unknown): value is StoredViewerSessionPreferences {
  if (!value || typeof value !== 'object') return false;
  const parsed = value as Record<string, unknown>;
  if (parsed.version !== 1) return false;
  if (parsed.preferOriginal !== undefined && typeof parsed.preferOriginal !== 'boolean') return false;
  if (
    parsed.rotation !== undefined &&
    parsed.rotation !== 0 &&
    parsed.rotation !== 90 &&
    parsed.rotation !== 180 &&
    parsed.rotation !== 270
  ) return false;
  if (
    parsed.fitMode !== undefined &&
    parsed.fitMode !== 'screen' &&
    parsed.fitMode !== 'fit_window' &&
    parsed.fitMode !== 'fit_down_only' &&
    parsed.fitMode !== 'original_size_if_fit' &&
    parsed.fitMode !== 'actual'
  ) return false;
  if (parsed.scaling !== undefined && parsed.scaling !== 'smooth' && parsed.scaling !== 'nearest') return false;
  return true;
}

function parseStoredPreferences(storage: SessionStorage | undefined): Partial<ViewerSessionPreferences> {
  const parsed = readBrowserSessionPreference<StoredViewerSessionPreferences | null>(
    viewerSessionStorageKey,
    null,
    (value): value is StoredViewerSessionPreferences | null => value === null || isStoredViewerSessionPreferences(value),
    storage
  );
  if (!parsed) return {};

  const preferences: Partial<ViewerSessionPreferences> = {};
  if (typeof parsed.preferOriginal === 'boolean') preferences.preferOriginal = parsed.preferOriginal;
  if (parsed.rotation === 0 || parsed.rotation === 90 || parsed.rotation === 180 || parsed.rotation === 270) {
    preferences.rotation = parsed.rotation;
  }
  if (parsed.fitMode === 'screen') preferences.fitMode = 'fit_window';
  else if (parsed.fitMode === 'fit_window' || parsed.fitMode === 'fit_down_only' || parsed.fitMode === 'original_size_if_fit' || parsed.fitMode === 'actual') preferences.fitMode = parsed.fitMode;
  if (parsed.scaling === 'smooth' || parsed.scaling === 'nearest') preferences.scaling = parsed.scaling;
  return preferences;
}

export function readViewerSessionPreferences(
  defaults: ViewerSessionPreferences,
  storage?: SessionStorage
): ViewerSessionPreferences {
  return { ...defaults, ...parseStoredPreferences(storage) };
}

export function updateViewerSessionPreferences(
  patch: Partial<ViewerSessionPreferences>,
  storage?: SessionStorage
) {
  // Keep this persisted shape a deliberately closed allowlist of low-sensitivity UI preferences.
  // Do not add file/query/library identity or other content-derived state here.
  const next: StoredViewerSessionPreferences = {
    version: 1,
    ...parseStoredPreferences(storage),
    ...patch
  };
  writeBrowserSessionPreference(viewerSessionStorageKey, next, storage);
}
