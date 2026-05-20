import type { ApiErrorResponse, FileListResponse } from './types';

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
  readonly token: string;

  constructor(token: string, baseURL = '/api/v1') {
    this.baseURL = baseURL;
    this.token = token;
  }

  async listFiles(params: ListFilesParams = {}): Promise<FileListResponse> {
    const url = new URL(`${this.baseURL}/files`, globalThis.location?.origin ?? 'http://localhost');
    if (params.query) url.searchParams.set('query', params.query);
    if (params.limit) url.searchParams.set('limit', String(params.limit));
    if (params.pageToken) url.searchParams.set('page_token', params.pageToken);
    return this.request<FileListResponse>(url.pathname + url.search);
  }

  private async request<T>(path: string): Promise<T> {
    const response = await fetch(path, {
      headers: {
        Accept: 'application/json',
        Authorization: `Bearer ${this.token}`
      }
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
