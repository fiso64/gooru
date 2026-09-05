import type { ViewerFitMode } from '$lib/utils/viewer';

export type ViewerRotation = 0 | 90 | 180 | 270;

export interface ViewerSessionPreferences {
  preferOriginal: boolean;
  rotation: ViewerRotation;
  fitMode: ViewerFitMode;
}

type StoredViewerSessionPreferences = Partial<ViewerSessionPreferences> & { version: 1 };
type SessionStorage = Pick<Storage, 'getItem' | 'setItem'>;

export const viewerSessionStorageKey = 'gooru.viewer.preferences.v1';

function browserSessionStorage(): SessionStorage | undefined {
  if (typeof window === 'undefined') return undefined;
  try {
    return window.sessionStorage;
  } catch {
    return undefined;
  }
}

function parseStoredPreferences(storage: SessionStorage | undefined): Partial<ViewerSessionPreferences> {
  if (!storage) return {};
  let raw: string | null;
  try {
    raw = storage.getItem(viewerSessionStorageKey);
  } catch {
    return {};
  }
  if (!raw) return {};

  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    if (parsed.version !== 1) return {};

    const preferences: Partial<ViewerSessionPreferences> = {};
    if (typeof parsed.preferOriginal === 'boolean') preferences.preferOriginal = parsed.preferOriginal;
    if (parsed.rotation === 0 || parsed.rotation === 90 || parsed.rotation === 180 || parsed.rotation === 270) {
      preferences.rotation = parsed.rotation;
    }
    if (parsed.fitMode === 'screen') preferences.fitMode = 'fit_window';
    else if (parsed.fitMode === 'fit_window' || parsed.fitMode === 'fit_down_only' || parsed.fitMode === 'original_size_if_fit' || parsed.fitMode === 'actual') preferences.fitMode = parsed.fitMode;
    return preferences;
  } catch {
    return {};
  }
}

export function readViewerSessionPreferences(
  defaults: ViewerSessionPreferences,
  storage: SessionStorage | undefined = browserSessionStorage()
): ViewerSessionPreferences {
  return { ...defaults, ...parseStoredPreferences(storage) };
}

export function updateViewerSessionPreferences(
  patch: Partial<ViewerSessionPreferences>,
  storage: SessionStorage | undefined = browserSessionStorage()
) {
  if (!storage) return;

  // Keep this persisted shape a deliberately closed allowlist of low-sensitivity UI preferences.
  // Do not add file/query/library identity or other content-derived state here.
  const next: StoredViewerSessionPreferences = {
    version: 1,
    ...parseStoredPreferences(storage),
    ...patch
  };
  try {
    storage.setItem(viewerSessionStorageKey, JSON.stringify(next));
  } catch {
    // Session persistence is a convenience; storage denial/quota must not break the viewer.
  }
}
