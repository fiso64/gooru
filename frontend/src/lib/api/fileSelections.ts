import { ApiError } from '$lib/api/client';
import type { ApiErrorResponse } from '$lib/api/types';

export interface FileSelectionSnapshot {
  id: string;
  count: number;
}

export async function createFileSelection(csrfToken: string, query: string, signal?: AbortSignal): Promise<FileSelectionSnapshot> {
  return requestJSON<FileSelectionSnapshot>('/api/v1/file-selections', csrfToken, {
    method: 'POST',
    signal,
    body: JSON.stringify({ query: query.trim() })
  });
}

export async function fileSelectionMembers(csrfToken: string, selectionID: string, fileIDs: string[], signal?: AbortSignal): Promise<string[]> {
  if (!fileIDs.length) return [];
  const response = await requestJSON<{ file_ids?: string[] }>(`/api/v1/file-selections/${encodeURIComponent(selectionID)}/members`, csrfToken, {
    method: 'POST',
    signal,
    body: JSON.stringify({ file_ids: fileIDs })
  });
  return response.file_ids ?? [];
}

export async function deleteFileSelection(csrfToken: string, selectionID: string): Promise<void> {
  if (!selectionID) return;
  const response = await fetch(`/api/v1/file-selections/${encodeURIComponent(selectionID)}`, {
    method: 'DELETE',
    credentials: 'same-origin',
    headers: { 'X-Gooru-CSRF': csrfToken }
  });
  if (response.ok || response.status === 404 || response.status === 410) return;
  const payload = await parseJSON<ApiErrorResponse>(response);
  throw apiError(response, payload);
}

async function requestJSON<T>(path: string, csrfToken: string, init: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: 'same-origin',
    headers: {
      'Content-Type': 'application/json',
      'X-Gooru-CSRF': csrfToken,
      ...(init.headers ?? {})
    }
  });
  const payload = await parseJSON<T | ApiErrorResponse>(response);
  if (!response.ok || !payload) throw apiError(response, payload);
  return payload as T;
}

async function parseJSON<T>(response: Response): Promise<T | undefined> {
  try {
    return await response.json() as T;
  } catch {
    return undefined;
  }
}

function apiError(response: Response, payload: unknown): ApiError {
  const errorPayload = payload as ApiErrorResponse | undefined;
  return new ApiError(
    response.status,
    errorPayload?.error.code ?? 'http_error',
    errorPayload?.error.message ?? `Request failed with HTTP ${response.status}`
  );
}
