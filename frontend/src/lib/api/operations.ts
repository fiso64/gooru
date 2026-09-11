import { ApiError } from '$lib/api/client';
import type { ApiErrorResponse, Job } from '$lib/api/types';

export interface BackgroundOperation {
  id: string;
  kind: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'canceled';
  stage?: 'receiving' | 'importing';
  progress_total: number;
  progress_completed: number;
  progress_completed_prefix?: number;
  progress_failed: number;
  progress?: number;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  error_code?: string;
  error_message?: string;
  result?: unknown;
}

interface BackgroundOperationListResponse {
  items: BackgroundOperation[];
  active_count?: number;
}

async function operationRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    credentials: 'same-origin',
    headers: { Accept: 'application/json', ...init?.headers },
    ...init
  });
  const payload = await response.json().catch(() => undefined) as (T & Partial<ApiErrorResponse>) | undefined;
  if (!response.ok) {
    throw new ApiError(
      response.status,
      payload?.error?.code ?? 'http_error',
      payload?.error?.message ?? `Request failed with HTTP ${response.status}`
    );
  }
  if (payload === undefined) throw new Error('Background operation response did not contain JSON');
  return payload;
}

export function listBackgroundOperations(limit = 1000) {
  const params = new URLSearchParams({ limit: String(limit) });
  return operationRequest<BackgroundOperationListResponse>(`/api/v1/operations?${params.toString()}`);
}

export function listBackgroundOperationsByIDs(ids: string[]) {
  const params = new URLSearchParams();
  for (const id of ids) params.append('id', id);
  return operationRequest<BackgroundOperationListResponse>(`/api/v1/operations?${params.toString()}`);
}

export function cancelBackgroundOperation(id: string, csrfToken: string) {
  return operationRequest<BackgroundOperation>(`/api/v1/operations/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: csrfToken ? { 'X-Gooru-CSRF': csrfToken } : undefined
  });
}

export function backgroundOperationAsJob(operation: BackgroundOperation): Job {
  const total = Math.max(0, operation.progress_total);
  const completed = Math.max(0, operation.progress_completed + operation.progress_failed);
  const fallbackProgress = total > 1
    ? Math.max(0, Math.min(1, completed / total))
    : undefined;
  const progress = operation.progress !== undefined
    ? Math.max(0, Math.min(1, operation.progress))
    : operation.kind === 'upload_import'
      ? undefined
      : fallbackProgress;

  return {
    id: operation.id,
    type: operation.kind,
    status: operation.status,
    stage: operation.stage,
    progress,
    progress_total: operation.progress_total,
    progress_completed: operation.progress_completed,
    progress_completed_prefix: operation.progress_completed_prefix,
    progress_failed: operation.progress_failed,
    submitted_at: operation.created_at,
    started_at: operation.started_at,
    finished_at: operation.finished_at,
    result: operation.result,
    error: operation.error_message
  };
}
