import type { ApiErrorResponse, AuthMeResponse, FileListResponse, Job, TagMutationOperation, TagMutationRequest, TagMutationResponse, UploadImportResponse } from './types';

export class ApiError extends Error {
  code: string;
  status: number;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }
}

export interface ListFilesParams {
  query?: string;
  limit?: number;
  pageToken?: string;
}

export class ApiClient {
  readonly baseURL: string;
  readonly csrfToken: string;

  constructor(csrfToken = '', baseURL = '/api/v1') {
    this.baseURL = baseURL;
    this.csrfToken = csrfToken;
  }

  async login(username: string, password: string): Promise<AuthMeResponse> {
    return this.request<AuthMeResponse>(`${this.baseURL}/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password })
    });
  }

  async me(): Promise<AuthMeResponse> {
    return this.request<AuthMeResponse>(`${this.baseURL}/auth/me`);
  }

  async logout(): Promise<void> {
    await this.request<{ ok: boolean }>(`${this.baseURL}/auth/logout`, {
      method: 'POST'
    });
  }

  async listFiles(params: ListFilesParams = {}): Promise<FileListResponse> {
    const url = new URL(`${this.baseURL}/files`, globalThis.location?.origin ?? 'http://localhost');
    if (params.query) url.searchParams.set('query', params.query);
    if (params.limit) url.searchParams.set('limit', String(params.limit));
    if (params.pageToken) url.searchParams.set('page_token', params.pageToken);
    return this.request<FileListResponse>(url.pathname + url.search);
  }

  async mutateTags(operation: TagMutationOperation, body: TagMutationRequest): Promise<TagMutationResponse> {
    const method = operation === 'add' ? 'POST' : operation === 'set' ? 'PUT' : 'DELETE';
    return this.request<TagMutationResponse>(`${this.baseURL}/files/tags`, {
      method,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
  }

  async uploadFiles(files: File[], tags: string[] = [], preferAsync = true): Promise<Job | UploadImportResponse> {
    const form = new FormData();
    for (const file of files) form.append('files', file, file.name);
    if (tags.length) form.append('tags', tags.join(' '));
    return this.request<Job | UploadImportResponse>(`${this.baseURL}/uploads`, {
      method: 'POST',
      headers: preferAsync ? { Prefer: 'respond-async' } : undefined,
      body: form
    });
  }

  async getJob(id: string): Promise<Job> {
    return this.request<Job>(`${this.baseURL}/jobs/${id}`);
  }

  async cancelJob(id: string): Promise<Job> {
    return this.request<Job>(`${this.baseURL}/jobs/${id}`, {
      method: 'DELETE'
    });
  }

  private async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const headers = new Headers(init.headers);
    headers.set('Accept', 'application/json');
    if (this.csrfToken && isMutatingMethod(init.method ?? 'GET')) {
      headers.set('X-Gooru-CSRF', this.csrfToken);
    }
    const response = await fetch(path, {
      ...init,
      headers,
      credentials: 'same-origin'
    });
    if (!response.ok) {
      let payload: ApiErrorResponse | undefined;
      try {
        payload = (await response.json()) as ApiErrorResponse;
      } catch {
        // Keep the fallback below.
      }
      throw new ApiError(
        response.status,
        payload?.error.code ?? 'http_error',
        payload?.error.message ?? `Request failed with HTTP ${response.status}`
      );
    }
    return (await response.json()) as T;
  }
}

function isMutatingMethod(method: string): boolean {
  return ['POST', 'PUT', 'PATCH', 'DELETE'].includes(method.toUpperCase());
}
