import { ApiError } from './client';
import type { BackgroundOperation } from './operations';
import type { ApiErrorResponse } from './types';

export interface SegmentedUploadRequest {
  files: File[];
  tags: string[];
  itemTags?: string[][];
  targetID: string;
  conflictPolicy: string;
  addedAtStrategy?: 'queue' | 'reverse_queue' | 'modtime';
  queueTimeMs?: number[];
  queueFirstTimeMs?: number;
  queueLastTimeMs?: number;
  queueIndex?: number[];
  queueTotal?: number[];
  operationID?: string;
  segmentIndex: number;
  segmentCount: number;
  onProgress?: (progress: number) => void;
  signal?: AbortSignal;
}

export async function uploadSegmentedFiles(
  csrfToken: string,
  request: SegmentedUploadRequest,
  baseURL = '/api/v1'
): Promise<BackgroundOperation> {
  if (request.itemTags !== undefined && request.itemTags.length !== request.files.length) {
    throw new ApiError(0, 'invalid_upload_item_tags', 'Per-file upload tags must align with uploaded files');
  }
  if (!Number.isInteger(request.segmentCount) || request.segmentCount <= 1) {
    throw new ApiError(0, 'invalid_upload_segment_count', 'Segmented uploads require more than one segment');
  }
  if (!Number.isInteger(request.segmentIndex) || request.segmentIndex < 0 || request.segmentIndex >= request.segmentCount) {
    throw new ApiError(0, 'invalid_upload_segment_index', 'Upload segment index is out of range');
  }

  const absoluteBase = absoluteBaseURL(baseURL);
  let operationID = request.operationID;
  if (!operationID) {
    const reservation = await reserveSegmentedUpload(absoluteBase, csrfToken, request.segmentCount);
    operationID = reservation.id;
  }
  const cancelOperation = () => cancelReservedUpload(absoluteBase, csrfToken, operationID!);
  if (request.signal?.aborted) {
    await cancelOperation();
    throw new ApiError(0, 'request_aborted', 'Upload was canceled');
  }

  const form = segmentedUploadForm(request);
  return uploadSegmentMultipart(`${absoluteBase}/uploads`, form, {
    baseURL: absoluteBase,
    csrfToken,
    operationID,
    segmentIndex: request.segmentIndex,
    onProgress: request.onProgress,
    signal: request.signal,
    cancelOperation
  });
}

function segmentedUploadForm(request: SegmentedUploadRequest): FormData {
  const form = new FormData();
  for (const file of request.files) form.append('files', file, file.name);
  for (const file of request.files) form.append('source_modtime_ms', String(file.lastModified));
  if (request.tags.length) form.append('tags', request.tags.join(' '));
  for (const itemTags of request.itemTags ?? []) form.append('item_tags', itemTags.join(' '));
  if (request.targetID) form.append('target_id', request.targetID);
  if (request.conflictPolicy) form.append('conflict_policy', request.conflictPolicy);
  if (request.addedAtStrategy) form.append('added_at_strategy', request.addedAtStrategy);
  for (const value of request.queueTimeMs ?? []) if (Number.isFinite(value) && value > 0) form.append('queue_time_ms', String(Math.trunc(value)));
  if (Number.isFinite(request.queueFirstTimeMs) && (request.queueFirstTimeMs ?? 0) > 0) form.append('queue_first_time_ms', String(Math.trunc(request.queueFirstTimeMs!)));
  if (Number.isFinite(request.queueLastTimeMs) && (request.queueLastTimeMs ?? 0) > 0) form.append('queue_last_time_ms', String(Math.trunc(request.queueLastTimeMs!)));
  const queueIndices = request.queueIndex ?? [];
  const queueTotals = request.queueTotal ?? [];
  const canonicalIndices = queueIndices.length === request.files.length && queueIndices.every((value, index) => value === index);
  const canonicalTotals = queueTotals.length === request.files.length && queueTotals.every((value) => value === request.files.length);
  if (!canonicalIndices) {
    for (const value of queueIndices) if (Number.isInteger(value) && value >= 0) form.append('queue_index', String(value));
  }
  if (!canonicalTotals) {
    for (const value of queueTotals) if (Number.isInteger(value) && value > 0) form.append('queue_total', String(value));
  }
  return form;
}

async function reserveSegmentedUpload(baseURL: string, csrfToken: string, segmentCount: number): Promise<BackgroundOperation> {
  const response = await fetch(`${baseURL}/uploads`, {
    method: 'POST',
    credentials: 'same-origin',
    headers: {
      'X-Gooru-CSRF': csrfToken,
      'X-Gooru-Upload-Reserve': 'true',
      'X-Gooru-Upload-Segment-Count': String(segmentCount)
    }
  });
  const payload = await parseJSONResponse<BackgroundOperation>(response);
  if (!response.ok || !payload?.id) throw apiErrorFromResponse(response, payload);
  return payload;
}

async function cancelReservedUpload(baseURL: string, csrfToken: string, operationID: string): Promise<void> {
  try {
    await fetch(`${baseURL}/operations/${encodeURIComponent(operationID)}`, {
      method: 'DELETE',
      credentials: 'same-origin',
      headers: { 'X-Gooru-CSRF': csrfToken }
    });
  } catch {
    // Best effort: server-side reservation recovery also reclaims abandoned staging.
  }
}

async function uploadOperationWasCanceled(baseURL: string, operationID: string): Promise<boolean> {
  try {
    const params = new URLSearchParams({ id: operationID });
    const response = await fetch(`${baseURL}/operations?${params.toString()}`, {
      credentials: 'same-origin',
      headers: { Accept: 'application/json' }
    });
    if (!response.ok) return false;
    const payload = await parseJSONResponse<{ items?: BackgroundOperation[] }>(response);
    return payload?.items?.some((operation) => operation.id === operationID && operation.status === 'canceled') ?? false;
  } catch {
    return false;
  }
}

interface SegmentMultipartOptions {
  baseURL: string;
  csrfToken: string;
  operationID: string;
  segmentIndex: number;
  onProgress?: (progress: number) => void;
  signal?: AbortSignal;
  cancelOperation: () => Promise<void>;
}

function uploadSegmentMultipart(url: string, form: FormData, options: SegmentMultipartOptions): Promise<BackgroundOperation> {
  return new Promise<BackgroundOperation>((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', url);
    xhr.withCredentials = true;
    if (options.csrfToken) xhr.setRequestHeader('X-Gooru-CSRF', options.csrfToken);
    xhr.setRequestHeader('Prefer', 'respond-async');
    xhr.setRequestHeader('X-Gooru-Upload-Operation-ID', options.operationID);
    xhr.setRequestHeader('X-Gooru-Upload-Segment-Index', String(options.segmentIndex));

    xhr.upload.addEventListener('progress', (event) => {
      if (!event.lengthComputable || event.total <= 0) return;
      options.onProgress?.(Math.max(0, Math.min(100, Math.round((event.loaded / event.total) * 100))));
    });

    const cancelOperation = () => { void options.cancelOperation(); };
    xhr.addEventListener('load', () => {
      const payload = parseXHRPayload(xhr.responseText);
      if (xhr.status >= 200 && xhr.status < 300) {
        options.onProgress?.(100);
        resolve(payload as BackgroundOperation);
        return;
      }
      cancelOperation();
      const errorPayload = payload as ApiErrorResponse | undefined;
      reject(new ApiError(
        xhr.status,
        errorPayload?.error?.code ?? 'http_error',
        errorPayload?.error?.message ?? `Request failed with HTTP ${xhr.status}`
      ));
    });
    xhr.addEventListener('error', () => {
      void (async () => {
        if (await uploadOperationWasCanceled(options.baseURL, options.operationID)) {
          reject(new ApiError(0, 'request_aborted', 'Upload was canceled'));
          return;
        }
        cancelOperation();
        reject(new ApiError(0, 'network_error', 'Network error while uploading files'));
      })();
    });
    xhr.addEventListener('abort', () => {
      cancelOperation();
      reject(new ApiError(0, 'request_aborted', 'Upload was canceled'));
    });

    if (options.signal?.aborted) {
      cancelOperation();
      reject(new ApiError(0, 'request_aborted', 'Upload was canceled'));
      return;
    }
    const abortUpload = () => {
      cancelOperation();
      xhr.abort();
    };
    options.signal?.addEventListener('abort', abortUpload, { once: true });
    xhr.addEventListener('loadend', () => options.signal?.removeEventListener('abort', abortUpload), { once: true });
    xhr.send(form);
  });
}

async function parseJSONResponse<T>(response: Response): Promise<T | undefined> {
  try { return await response.json() as T; } catch { return undefined; }
}

function apiErrorFromResponse(response: Response, payload: unknown): ApiError {
  const errorPayload = payload as ApiErrorResponse | undefined;
  return new ApiError(
    response.status,
    errorPayload?.error?.code ?? 'http_error',
    errorPayload?.error?.message ?? `Request failed with HTTP ${response.status}`
  );
}

function parseXHRPayload(text: string): unknown {
  if (!text) return undefined;
  try { return JSON.parse(text); } catch { return undefined; }
}

function absoluteBaseURL(baseURL: string): string {
  const origin = globalThis.location?.origin ?? 'http://localhost';
  return new URL(baseURL, origin).href.replace(/\/$/, '');
}
