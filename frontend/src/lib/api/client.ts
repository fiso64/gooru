import { absoluteBaseURL } from '$lib/api/baseURL';
import { useProtectedReadTransport } from '$lib/api/privacy';
import { apiErrorFromResponse, parseJSONResponse } from '$lib/api/response';
import type { components } from '$lib/api/schema';
import type { ComicManifest, FileItem, Job, SavedSearch, SavedSearchRequest, SuggestionsResponse, TagFacetResponse, TagsResponse, UploadBatchResponse, User } from '$lib/api/types';
import type { FileSort, SortOrder } from '$lib/queries/files';
import type { LibraryURLState } from '$lib/utils/appRoute';
import createClient from 'openapi-fetch';
import type { paths } from '$lib/api/schema';

export class ApiClient {
  private readonly client;

  constructor(private readonly csrfToken = '', private readonly baseURL = '/api') {
    this.client = createClient<paths>({ baseUrl: absoluteBaseURL(baseURL), credentials: 'same-origin' });
  }

  private async unwrap<T>(request: Promise<{ data?: T; error?: unknown; response: Response }>): Promise<T> {
    const { data, error, response } = await request;
    if (!response.ok || error !== undefined || data === undefined) throw apiErrorFromResponse(response, error);
    return data;
  }

  async bootstrap(signal?: AbortSignal): Promise<User> {
    return this.unwrap(this.client.GET('/auth/me', { signal }));
  }

  async login(username: string, password: string): Promise<User> {
    return this.unwrap(this.client.POST('/auth/login', { body: { username, password } }));
  }

  async logout(): Promise<void> {
    await this.unwrap(this.client.POST('/auth/logout', { headers: { 'X-Gooru-CSRF': this.csrfToken } }));
  }

  async changePassword(currentPassword: string, newPassword: string): Promise<void> {
    await this.unwrap(this.client.POST('/auth/password', { headers: { 'X-Gooru-CSRF': this.csrfToken }, body: { current_password: currentPassword, new_password: newPassword } }));
  }

  async getFiles(params: {
    q?: string;
    limit?: number;
    cursor?: string;
    direction?: 'next' | 'prev';
    sort?: FileSort;
    order?: SortOrder;
    offset?: number;
    signal?: AbortSignal;
  }) {
    const body = {
      q: params.q || undefined,
      limit: params.limit,
      cursor: params.cursor,
      direction: params.direction,
      sort: params.sort,
      order: params.order,
      offset: params.offset
    };
    if (useProtectedReadTransport()) return this.unwrap(this.client.POST('/search/files', { body, signal: params.signal }));
    return this.unwrap(
      this.client.GET('/files', {
        params: {
          query: {
            q: body.q,
            limit: body.limit,
            cursor: body.cursor,
            direction: body.direction,
            sort: body.sort,
            order: body.order,
            offset: body.offset
          }
        },
        signal: params.signal
      })
    );
  }

  async getTags(params: { q?: string; limit?: number; offset?: number; signal?: AbortSignal }): Promise<TagsResponse> {
    const body = { q: params.q || undefined, limit: params.limit, offset: params.offset };
    if (useProtectedReadTransport()) return this.unwrap(this.client.POST('/search/tags', { body, signal: params.signal }));
    return this.unwrap(this.client.GET('/tags', { params: { query: body }, signal: params.signal }));
  }

  async getFileFacets(q = '', signal?: AbortSignal): Promise<TagFacetResponse> {
    if (useProtectedReadTransport()) return this.unwrap(this.client.POST('/search/facets', { body: { q: q || undefined }, signal }));
    return this.unwrap(this.client.GET('/facets', { params: { query: { q: q || undefined } }, signal }));
  }

  async createURLState(state: LibraryURLState, signal?: AbortSignal): Promise<string> {
    const response = await fetch(`${absoluteBaseURL(this.baseURL)}/ui-state`, {
      method: 'POST', credentials: 'same-origin', signal,
      headers: { 'Content-Type': 'application/json', 'X-Gooru-CSRF': this.csrfToken },
      body: JSON.stringify({ query: state.query, kind: state.kind, sort: state.sort, order: state.order, file_id: state.fileID, page: state.page })
    });
    const payload = await parseJSONResponse<{ token?: string }>(response);
    if (!response.ok || !payload?.token) throw apiErrorFromResponse(response, payload);
    return payload.token;
  }

  async resolveURLState(token: string, signal?: AbortSignal): Promise<LibraryURLState> {
    const response = await fetch(`${absoluteBaseURL(this.baseURL)}/ui-state/${encodeURIComponent(token)}`, { credentials: 'same-origin', signal });
    const payload = await parseJSONResponse<{ query?: string; kind?: string; sort?: FileSort; order?: SortOrder; file_id?: string; page?: number }>(response);
    if (!response.ok || !payload) throw apiErrorFromResponse(response, payload);
    return { query: payload.query ?? '', kind: payload.kind ?? '', sort: payload.sort ?? 'added', order: payload.order ?? 'desc', fileID: payload.file_id ?? '', page: payload.page ?? 1 };
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
      this.client.GET('/suggestions', {
        params: { query: { q: q || undefined, limit, existing: existing || undefined } },
        signal
      })
    );
  }

  async updateFileTags(id: string, tags: string[]): Promise<FileItem> {
    return this.unwrap(this.client.PATCH('/files/{id}/tags', { params: { path: { id } }, headers: { 'X-Gooru-CSRF': this.csrfToken }, body: { tags } }));
  }

  async deleteFile(id: string): Promise<void> {
    await this.unwrap(this.client.DELETE('/files/{id}', { params: { path: { id } }, headers: { 'X-Gooru-CSRF': this.csrfToken } }));
  }

  async untrackFile(id: string): Promise<void> {
    await this.unwrap(this.client.DELETE('/files/{id}/tracking', { params: { path: { id } }, headers: { 'X-Gooru-CSRF': this.csrfToken } }));
  }

  async uploadFiles(files: File[], target: string, conflict: 'skip' | 'rename' | 'replace', onProgress?: (loaded: number, total: number) => void): Promise<UploadBatchResponse> {
    const form = new FormData();
    for (const file of files) form.append('files', file);
    form.append('target', target);
    form.append('conflict', conflict);
    return this.uploadForm(form, onProgress);
  }

  private uploadForm(form: FormData, onProgress?: (loaded: number, total: number) => void): Promise<UploadBatchResponse> {
    return new Promise((resolve, reject) => {
      const request = new XMLHttpRequest();
      request.open('POST', `${absoluteBaseURL(this.baseURL)}/uploads`);
      request.withCredentials = true;
      request.setRequestHeader('X-Gooru-CSRF', this.csrfToken);
      request.upload.onprogress = (event) => onProgress?.(event.loaded, event.lengthComputable ? event.total : 0);
      request.onerror = () => reject(new Error('Upload failed'));
      request.onload = async () => {
        let payload: UploadBatchResponse | null = null;
        try { payload = request.responseText ? JSON.parse(request.responseText) as UploadBatchResponse : null; } catch { /* response helper below */ }
        if (request.status < 200 || request.status >= 300 || !payload) {
          reject(new Error(payload && 'error' in payload ? String((payload as { error?: string }).error ?? 'Upload failed') : `Upload failed (${request.status})`));
          return;
        }
        resolve(payload);
      };
      request.send(form);
    });
  }

  async getUploadTargets(signal?: AbortSignal) {
    return this.unwrap(this.client.GET('/upload-targets', { signal }));
  }

  async getJobs(signal?: AbortSignal): Promise<Job[]> {
    return this.unwrap(this.client.GET('/jobs', { signal }));
  }

  async cancelJob(id: string): Promise<Job> {
    return this.unwrap(this.client.POST('/jobs/{id}/cancel', { params: { path: { id } }, headers: { 'X-Gooru-CSRF': this.csrfToken } }));
  }

  async listSavedSearches(signal?: AbortSignal): Promise<SavedSearch[]> {
    return this.unwrap(this.client.GET('/saved-searches', { signal }));
  }

  async createSavedSearch(request: SavedSearchRequest): Promise<SavedSearch> {
    return this.unwrap(this.client.POST('/saved-searches', { headers: { 'X-Gooru-CSRF': this.csrfToken }, body: request }));
  }

  async updateSavedSearch(id: string, request: SavedSearchRequest): Promise<SavedSearch> {
    return this.unwrap(this.client.PUT('/saved-searches/{id}', { params: { path: { id } }, headers: { 'X-Gooru-CSRF': this.csrfToken }, body: request }));
  }

  async deleteSavedSearch(id: string): Promise<void> {
    await this.unwrap(this.client.DELETE('/saved-searches/{id}', { params: { path: { id } }, headers: { 'X-Gooru-CSRF': this.csrfToken } }));
  }

  async reorderSavedSearches(ids: string[]): Promise<SavedSearch[]> {
    return this.unwrap(this.client.PUT('/saved-searches/order', { headers: { 'X-Gooru-CSRF': this.csrfToken }, body: { ids } }));
  }
}
