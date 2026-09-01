import createClient from 'openapi-fetch';
import type { paths } from './openapi';
import type {
  ApiErrorResponse,
  AuthMeResponse,
  FileListResponse,
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

type FileSort = 'name' | 'modified' | 'size' | 'kind';
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

  async listFiles(params: ListFilesParams = {}): Promise<FileListResponse> {
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

  async searchSuggestions(q = '', limit?: number, existing = '', signal?: AbortSignal): Promise<SuggestionsResponse> {
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

  async deleteSavedSearch(id: string): Promise<void> {
    await this.unwrap(this.client.DELETE('/saved-searches/{id}', { params: { header: this.csrfHeaderParam('DELETE'), path: { id } } }));
  }

  async mutateTags(operation: TagMutationOperation, body: TagMutationRequest): Promise<TagMutationResponse> {
    const requestBody = tagMutationBody(body);
    if (operation === 'add') {
      return this.unwrap<TagMutationResponse>(this.client.POST('/files/tags', { headers: this.csrfHeaders('POST'), body: requestBody }));
    }
    if (operation === 'set') {
      return this.unwrap<TagMutationResponse>(this.client.PUT('/files/tags', { headers: this.csrfHeaders('PUT'), body: requestBody }));
    }
    return this.unwrap<TagMutationResponse>(this.client.DELETE('/files/tags', { headers: this.csrfHeaders('DELETE'), body: requestBody }));
  }

  async untrackFile(id: string): Promise<void> {
    await this.unwrap(
      this.client.DELETE('/files/{id}', {
        params: { header: this.csrfHeaderParam('DELETE'), path: { id } },
        body: { mode: 'untrack' }
      })
    );
  }

  async getUploadTargets(): Promise<UploadTargetsResponse> {
    return this.unwrap(this.client.GET('/upload-targets'));
  }

  async uploadFiles(files: File[], tags: string[] = [], preferAsync = true, targetID = '', conflictPolicy = 'rename'): Promise<Job | UploadImportResponse> {
    const form = new FormData();
    for (const file of files) form.append('files', file, file.name);
    if (tags.length) form.append('tags', tags.join(' '));
    if (targetID) form.append('target_id', targetID);
    if (conflictPolicy) form.append('conflict_policy', conflictPolicy);
    return this.unwrap(
      this.client.POST('/uploads', {
        headers: { ...this.csrfHeaders('POST'), ...(preferAsync ? { Prefer: 'respond-async' } : {}) },
        // openapi-fetch supports FormData, but the generated schema models multipart
        // fields structurally. Keep the browser-native body so filenames and blobs
        // are preserved exactly.
        body: form as never
      })
    );
  }

  async getJob(id: string): Promise<Job> {
    return this.unwrap(this.client.GET('/jobs/{id}', { params: { path: { id } } }));
  }

  async listJobs(status: JobStatus | '' = ''): Promise<JobListResponse> {
    return this.unwrap(this.client.GET('/jobs', { params: { query: { status: status || undefined } } }));
  }

  async cancelJob(id: string): Promise<Job> {
    return this.unwrap(this.client.DELETE('/jobs/{id}', { headers: this.csrfHeaders('DELETE'), params: { path: { id } } }));
  }

  async clearJobs(status = 'completed'): Promise<{ removed: number }> {
    const clearStatus = (['completed', 'failed', 'canceled'].includes(status) ? status : 'completed') as ClearableJobStatus;
    return this.unwrap(this.client.DELETE('/jobs', { params: { header: this.csrfHeaderParam('DELETE'), query: { status: clearStatus } } }));
  }

  private csrfHeaderParam(method: string): { 'X-Gooru-CSRF': string } {
    return { 'X-Gooru-CSRF': isMutatingMethod(method) ? this.csrfToken : '' };
  }

  private csrfHeaders(method: string): Record<string, string> {
    if (!this.csrfToken || !isMutatingMethod(method)) return {};
    return { 'X-Gooru-CSRF': this.csrfToken };
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
    sort: body.sort ?? 'name',
    order: body.order ?? 'asc'
  };
}

function tagMutationBody(body: TagMutationRequest) {
  return {
    ...body,
    verbose: body.verbose ?? false
  };
}
