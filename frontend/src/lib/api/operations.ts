import { ApiError } from '$lib/api/client';
import type { ApiErrorResponse, Job } from '$lib/api/types';

export interface BackgroundOperation {
  id: string;
  kind: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'canceled';
  outcome?: 'success' | 'partial_success' | 'error';
  affected_count?: number;
  failed_count?: number;
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
  total_count?: number;
}

export interface BackgroundOperationClearResponse {
  cleared: number;
}

export interface BackgroundOperationCancelAllResponse {
  canceled: number;
}

export interface MaintenanceJob {
  id: string;
  name: string;
  description: string;
  running: boolean;
}

export interface MaintenanceJobListResponse {
  items: MaintenanceJob[];
}

export interface MaintenanceJobRunResponse {
  job: MaintenanceJob;
  created: boolean;
}

async function jsonRequest<T>(path: string, init?: RequestInit): Promise<T> {
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
  if (payload === undefined) throw new Error('API response did not contain JSON');
  return payload;
}

export function listBackgroundOperations(limit = 1000, offset = 0) {
  const normalizedLimit = Math.max(1, Math.min(1000, Math.trunc(limit)));
  const normalizedOffset = Math.max(0, Math.trunc(offset));
  const params = new URLSearchParams({ limit: String(normalizedLimit), offset: String(normalizedOffset) });
  return jsonRequest<BackgroundOperationListResponse>(`/api/v1/operations?${params.toString()}`);
}

export function listBackgroundOperationsByIDs(ids: string[]) {
  const params = new URLSearchParams();
  for (const id of ids) params.append('id', id);
  return jsonRequest<BackgroundOperationListResponse>(`/api/v1/operations?${params.toString()}`);
}

export function clearCompletedBackgroundOperations(csrfToken: string) {
  return jsonRequest<BackgroundOperationClearResponse>('/api/v1/operations', {
    method: 'DELETE',
    headers: csrfToken ? { 'X-Gooru-CSRF': csrfToken } : undefined
  });
}

export function cancelActiveBackgroundOperations(csrfToken: string) {
  return jsonRequest<BackgroundOperationCancelAllResponse>('/api/v1/operations/cancel-all', {
    method: 'DELETE',
    headers: csrfToken ? { 'X-Gooru-CSRF': csrfToken } : undefined
  });
}

export function cancelBackgroundOperation(id: string, csrfToken: string) {
  return jsonRequest<BackgroundOperation>(`/api/v1/operations/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: csrfToken ? { 'X-Gooru-CSRF': csrfToken } : undefined
  });
}

export function listMaintenanceJobs() {
  return jsonRequest<MaintenanceJobListResponse>('/api/v1/maintenance-jobs');
}

export function runMaintenanceJob(id: string, csrfToken: string) {
  return jsonRequest<MaintenanceJobRunResponse>(`/api/v1/maintenance-jobs/${encodeURIComponent(id)}`, {
    method: 'POST',
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
    outcome: operation.outcome,
    affected_count: operation.affected_count,
    failed_count: operation.failed_count,
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
