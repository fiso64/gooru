import type { ApiErrorResponse, AuthMeResponse, FileListResponse, Job, JobListResponse, NamespacesResponse, SavedSearch, SavedSearchRequest, SavedSearchesResponse, SuggestionsResponse, TagListResponse, TagMutationOperation, TagMutationRequest, TagMutationResponse, UploadImportResponse, UploadTargetsResponse } from './types';

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
  sort?: 'name' | 'modified' | 'size' | 'kind';
  order?: 'asc' | 'desc';
  includeFacets?: boolean;
  signal?: AbortSignal;
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
    if (params.sort) url.searchParams.set('sort', params.sort);
    if (params.order) url.searchParams.set('order', params.order);
    if (params.includeFacets) url.searchParams.set('include_facets', 'true');
    return this.request<FileListResponse>(url.pathname + url.search, { signal: params.signal });
  }

  async searchSuggestions(q = '', limit?: number, existing = '', signal?: AbortSignal): Promise<SuggestionsResponse> {
    const url = new URL(`${this.baseURL}/search/suggestions`, globalThis.location?.origin ?? 'http://localhost');
    if (q) url.searchParams.set('q', q);
    if (existing) url.searchParams.set('existing', existing);
    if (limit) url.searchParams.set('limit', String(limit));
    return this.request<SuggestionsResponse>(url.pathname + url.search, { signal });
  }

  async tagNamespaces(): Promise<NamespacesResponse> {
    return this.request<NamespacesResponse>(`${this.baseURL}/tags/namespaces`);
  }

  async listTags(counts = true): Promise<TagListResponse> {
    const url = new URL(`${this.baseURL}/tags`, globalThis.location?.origin ?? 'http://localhost');
    if (counts) url.searchParams.set('counts', 'true');
    return this.request<TagListResponse>(url.pathname + url.search);
  }

  async listSavedSearches(): Promise<SavedSearchesResponse> {
    return this.request<SavedSearchesResponse>(`${this.baseURL}/saved-searches`);
  }

  async createSavedSearch(body: SavedSearchRequest): Promise<SavedSearch> {
    return this.request<SavedSearch>(`${this.baseURL}/saved-searches`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
  }

  async updateSavedSearch(id: string, body: SavedSearchRequest): Promise<SavedSearch> {
    return this.request<SavedSearch>(`${this.baseURL}/saved-searches/${encodeURIComponent(id)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
  }

  async deleteSavedSearch(id: string): Promise<void> {
    await this.request<{ ok: boolean }>(`${this.baseURL}/saved-searches/${encodeURIComponent(id)}`, {
      method: 'DELETE'
    });
  }

  async mutateTags(operation: TagMutationOperation, body: TagMutationRequest): Promise<TagMutationResponse> {
    const method = operation === 'add' ? 'POST' : operation === 'set' ? 'PUT' : 'DELETE';
    return this.request<TagMutationResponse>(`${this.baseURL}/files/tags`, {
      method,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
  }

  async getUploadTargets(): Promise<UploadTargetsResponse> {
    return this.request<UploadTargetsResponse>(`${this.baseURL}/upload-targets`);
  }

  async uploadFiles(files: File[], tags: string[] = [], preferAsync = true, targetID = ''): Promise<Job | UploadImportResponse> {
    const form = new FormData();
    for (const file of files) form.append('files', file, file.name);
    if (tags.length) form.append('tags', tags.join(' '));
    if (targetID) form.append('target_id', targetID);
    return this.request<Job | UploadImportResponse>(`${this.baseURL}/uploads`, {
      method: 'POST',
      headers: preferAsync ? { Prefer: 'respond-async' } : undefined,
      body: form
    });
  }

  async getJob(id: string): Promise<Job> {
    return this.request<Job>(`${this.baseURL}/jobs/${id}`);
  }

  async listJobs(status = ''): Promise<JobListResponse> {
    const url = new URL(`${this.baseURL}/jobs`, globalThis.location?.origin ?? 'http://localhost');
    if (status) url.searchParams.set('status', status);
    return this.request<JobListResponse>(url.pathname + url.search);
  }

  async cancelJob(id: string): Promise<Job> {
    return this.request<Job>(`${this.baseURL}/jobs/${id}`, {
      method: 'DELETE'
    });
  }

  async clearJobs(status = 'completed'): Promise<{ removed: number }> {
    const url = new URL(`${this.baseURL}/jobs`, globalThis.location?.origin ?? 'http://localhost');
    if (status) url.searchParams.set('status', status);
    return this.request<{ removed: number }>(url.pathname + url.search, {
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
