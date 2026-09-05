import createClient from 'openapi-fetch';
import type { paths } from './openapi';
import { useProtectedReadTransport } from './privacy';
import type {
  ApiErrorResponse,
  AuthMeResponse,
  ComicManifest,
  FileItem,
  FileListResponse,
  FileRemovalRequest,
  FileRemovalResponse,
  Job,
  JobListResponse,
  NamespacesResponse,
  SavedSearch,
  SavedSearchRequest,
  SavedSearchesResponse,
  SuggestionsResponse,
  TagListResponse,
  TagMutationOperation,
  TagMutationRequest,
  TagMutationResponse,
  UploadImportResponse,
  UploadTargetsResponse
} from './types';

type FileSort = 'added' | 'name' | 'modified' | 'size' | 'kind';
type SortOrder = 'asc' | 'desc';
type JobStatus = 'pending' | 'running' | 'completed' | 'failed' | 'canceled';
type ClearableJobStatus = 'completed' | 'failed' | 'canceled';

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

let unauthorizedHandler: (() => void) | undefined;

export function setUnauthorizedHandler(handler: (() => void) | undefined) {
  unauthorizedHandler = handler;
}

export interface ListFilesParams {
  query?: string;
  limit?: number;
  pageToken?: string;
  sort?: FileSort;
  order?: SortOrder;
  includeFacets?: boolean;
  signal?: AbortSignal;
}

export class ApiClient {
  readonly baseURL: string;
  readonly csrfToken: string;
  private readonly client: ReturnType<typeof createClient<paths>>;

  constructor(csrfToken = '', baseURL = '/api/v1') {
    this.baseURL = baseURL;
    this.csrfToken = csrfToken;
    this.client = createClient<paths>({ baseUrl: absoluteBaseURL(baseURL), credentials: 'same-origin', fetch: generatedFetch });
  }

  async login(username: string, password: string): Promise<AuthMeResponse> {
    return this.unwrap(this.client.POST('/auth/login', { body: { username, password } }));
  }

  async me(): Promise<AuthMeResponse> {
    return this.unwrap(this.client.GET('/auth/me'));
  }

  async logout(): Promise<void> {
    await this.unwrap(this.client.POST('/auth/logout', { params: { header: this.csrfHeaderParam('POST') } }));
  }

  async changePassword(currentPassword: string, newPassword: string): Promise<void> {
    await this.unwrap(
      this.client.POST('/auth/change-password', {
        params: { header: this.csrfHeaderParam('POST') },
        body: { current_password: currentPassword, new_password: newPassword }
      })
    );
  }

  async listFiles(params: ListFilesParams = {}): Promise<FileListResponse> {
    if (useProtectedReadTransport()) {
      return this.unwrap(
        this.client.POST('/files/search', {
          body: {
            query: params.query || undefined,
            limit: params.limit,
            page_token: params.pageToken,
            sort: params.sort,
            order: params.order,
            include_facets: params.includeFacets || undefined
          },
          signal: params.signal
        })
      );
    }
    return this.unwrap(
      this.client.GET('/files', {
        params: {
          query: {
            query: params.query || undefined,
            limit: params.limit,
            page_token: params.pageToken,
            sort: params.sort,
            order: params.order,
            include_facets: params.includeFacets || undefined
          }
        },
        signal: params.signal
      })
    );
  }

  async getFile(id: string, signal?: AbortSignal): Promise<FileItem> {
    return this.unwrap(this.client.GET('/files/{id}', { params: { path: { id } }, signal }));
  }

  async getComicManifest(id: string, signal?: AbortSignal): Promise<ComicManifest> {
    return this.unwrap(this.client.GET('/comics/{id}', { params: { path: { id } }, signal }));
  }

  async searchSuggestions(q = '', limit?: number, existing = '', signal?: AbortSignal): Promise<SuggestionsResponse> {
    if (useProtectedReadTransport()) {
      return this.unwrap(
        this.client.POST('/search/suggestions', { body: { q: q || undefined, limit, existing: existing || undefined }, signal })
      );
    }
    return this.unwrap(
      this.client.GET('/search/suggestions', {
        params: { query: { q: q || undefined, limit, existing: existing || undefined } },
        signal
      })
    );
  }

  async tagNamespaces(): Promise<NamespacesResponse> {
    return this.unwrap(this.client.GET('/tags/namespaces'));
  }

  async listTags(counts = true, limit = 200): Promise<TagListResponse> {
    return this.unwrap(this.client.GET('/tags', { params: { query: { counts, limit } } }));
  }

  async listSavedSearches(): Promise<SavedSearchesResponse> {
    return this.unwrap(this.client.GET('/saved-searches'));
  }

  async createSavedSearch(body: SavedSearchRequest): Promise<SavedSearch> {
    return this.unwrap<SavedSearch>(this.client.POST('/saved-searches', { params: { header: this.csrfHeaderParam('POST') }, body: savedSearchBody(body) }));
  }

  async updateSavedSearch(id: string, body: SavedSearchRequest): Promise<SavedSearch> {
    return this.unwrap<SavedSearch>(
      this.client.PUT('/saved-searches/{id}', { params: { header: this.csrfHeaderParam('PUT'), path: { id } }, body: savedSearchBody(body) })
    );
  }

  async reorderSavedSearches(ids: string[]): Promise<void> {
    await this.unwrap(
      this.client.PUT('/saved-searches/reorder', { params: { header: this.csrfHeaderParam('PUT') }, body: { ids } })
    );
  }

  async deleteSavedSearch(id: string): Promise<void> {
    await this.unwrap(this.client.DELETE('/saved-searches/{id}', { params: { header: this.csrfHeaderParam('DELETE'), path: { id } } }));
  }

  async mutateTags(operation: TagMutationOperation, body: TagMutationRequest): Promise<TagMutationResponse> {
    const requestBody = tagMutationBody(body);
    if (operation === 'add') {
      return this.unwrap<TagMutationResponse>(this.client.POST('/files/tags', { params: { header: this.csrfHeaderParam('POST') }, body: requestBody }));
    }
    if (operation === 'set') {
      return this.unwrap<TagMutationResponse>(this.client.PUT('/files/tags', { params: { header: this.csrfHeaderParam('PUT') }, body: requestBody }));
    }
    return this.unwrap<TagMutationResponse>(this.client.DELETE('/files/tags', { params: { header: this.csrfHeaderParam('DELETE') }, body: requestBody }));
  }

  async removeFile(id: string, mode: 'untrack' | 'delete' = 'untrack'): Promise<void> {
    await this.unwrap(
      this.client.DELETE('/files/{id}', {
        params: { header: this.csrfHeaderParam('DELETE'), path: { id } },
        body: { mode }
      })
    );
  }

  async removeFiles(body: FileRemovalRequest): Promise<FileRemovalResponse> {
    return this.unwrap<FileRemovalResponse>(
      this.client.DELETE('/files', { params: { header: this.csrfHeaderParam('DELETE') }, body })
    );
  }

  async getUploadTargets(): Promise<UploadTargetsResponse> {
    return this.unwrap(this.client.GET('/upload-targets'));
  }

  async uploadFiles(
    files: File[],
    tags: string[] = [],
    preferAsync = true,
    targetID = '',
    conflictPolicy = 'rename',
    onProgress?: (progress: number) => void,
    ordering: UploadOrderingMetadata = {}
  ): Promise<Job | UploadImportResponse> {
    const form = new FormData();
    for (const file of files) form.append('files', file, file.name);
    for (const file of files) form.append('source_modtime_ms', String(file.lastModified));
    if (tags.length) form.append('tags', tags.join(' '));
    if (targetID) form.append('target_id', targetID);
    if (conflictPolicy) form.append('conflict_policy', conflictPolicy);
    if (ordering.addedAtStrategy) form.append('added_at_strategy', ordering.addedAtStrategy);
    for (const value of ordering.queueTimeMs ?? []) if (Number.isFinite(value) && value > 0) form.append('queue_time_ms', String(Math.trunc(value)));
    if (Number.isFinite(ordering.queueFirstTimeMs) && (ordering.queueFirstTimeMs ?? 0) > 0) form.append('queue_first_time_ms', String(Math.trunc(ordering.queueFirstTimeMs!)));
    if (Number.isFinite(ordering.queueLastTimeMs) && (ordering.queueLastTimeMs ?? 0) > 0) form.append('queue_last_time_ms', String(Math.trunc(ordering.queueLastTimeMs!)));
    for (const value of ordering.queueIndex ?? []) if (Number.isInteger(value) && value >= 0) form.append('queue_index', String(value));
    for (const value of ordering.queueTotal ?? []) if (Number.isInteger(value) && value > 0) form.append('queue_total', String(value));

    return uploadMultipart<Job | UploadImportResponse>(`${absoluteBaseURL(this.baseURL)}/uploads`, form, {
      csrfToken: this.csrfToken,
      preferAsync,
      onProgress
    });
  }

  async getJob(id: string): Promise<Job> {
    return this.unwrap(this.client.GET('/jobs/{id}', { params: { path: { id } } }));
  }

  async listJobs(status: JobStatus | '' = ''): Promise<JobListResponse> {
    return this.unwrap(this.client.GET('/jobs', { params: { query: { status: status || undefined } } }));
  }

  async cancelJob(id: string): Promise<Job> {
    return this.unwrap(this.client.DELETE('/jobs/{id}', { params: { header: this.csrfHeaderParam('DELETE'), path: { id } } }));
  }

  async clearJobs(status = 'completed'): Promise<{ removed: number }> {
    const clearStatus = (['completed', 'failed', 'canceled'].includes(status) ? status : 'completed') as ClearableJobStatus;
    return this.unwrap(this.client.DELETE('/jobs', { params: { header: this.csrfHeaderParam('DELETE'), query: { status: clearStatus } } }));
  }

  private csrfHeaderParam(method: string): { 'X-Gooru-CSRF': string } {
    return { 'X-Gooru-CSRF': isMutatingMethod(method) ? this.csrfToken : '' };
  }

  private async unwrap<T>(request: Promise<{ data?: unknown; error?: unknown; response: Response }>): Promise<T> {
    const { data, error, response } = await request;
    if (!response.ok) {
      const payload = error as ApiErrorResponse | undefined;
      const apiError = new ApiError(
        response.status,
        payload?.error.code ?? 'http_error',
        payload?.error.message ?? `Request failed with HTTP ${response.status}`
      );
      if (response.status === 401) unauthorizedHandler?.();
      throw apiError;
    }
    return data as T;
  }
}

export interface UploadOrderingMetadata {
  addedAtStrategy?: 'queue' | 'reverse_queue' | 'modtime';
  queueTimeMs?: number[];
  queueFirstTimeMs?: number;
  queueLastTimeMs?: number;
  queueIndex?: number[];
  queueTotal?: number[];
}

interface UploadMultipartOptions {
  csrfToken: string;
  preferAsync: boolean;
  onProgress?: (progress: number) => void;
}

function uploadMultipart<T>(url: string, form: FormData, options: UploadMultipartOptions): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', url);
    xhr.withCredentials = true;
    if (options.csrfToken) xhr.setRequestHeader('X-Gooru-CSRF', options.csrfToken);
    if (options.preferAsync) xhr.setRequestHeader('Prefer', 'respond-async');

    xhr.upload.addEventListener('progress', (event) => {
      if (!event.lengthComputable || event.total <= 0) return;
      options.onProgress?.(Math.max(0, Math.min(100, Math.round((event.loaded / event.total) * 100))));
    });

    xhr.addEventListener('load', () => {
      const payload = parseXHRPayload(xhr.responseText);
      if (xhr.status >= 200 && xhr.status < 300) {
        options.onProgress?.(100);
        resolve(payload as T);
        return;
      }
      const errorPayload = payload as ApiErrorResponse | undefined;
      const apiError = new ApiError(
        xhr.status,
        errorPayload?.error?.code ?? 'http_error',
        errorPayload?.error?.message ?? `Request failed with HTTP ${xhr.status}`
      );
      if (xhr.status === 401) unauthorizedHandler?.();
      reject(apiError);
    });

    xhr.addEventListener('error', () => reject(new ApiError(0, 'network_error', 'Network error while uploading files')));
    xhr.addEventListener('abort', () => reject(new ApiError(0, 'request_aborted', 'Upload was canceled')));
    xhr.send(form);
  });
}

function parseXHRPayload(text: string): unknown {
  if (!text) return undefined;
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

function isMutatingMethod(method: string): boolean {
  return ['POST', 'PUT', 'PATCH', 'DELETE'].includes(method.toUpperCase());
}

function absoluteBaseURL(baseURL: string): string {
  const origin = globalThis.location?.origin ?? 'http://localhost';
  return new URL(baseURL, origin).href.replace(/\/$/, '');
}

function generatedFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  return fetch(input, init);
}

function savedSearchBody(body: SavedSearchRequest) {
  return {
    name: body.name,
    query: body.query,
    sort: body.sort ?? 'added',
    order: body.order ?? 'desc'
  };
}

function tagMutationBody(body: TagMutationRequest) {
  return {
    ...body,
    verbose: body.verbose ?? false
  };
}
