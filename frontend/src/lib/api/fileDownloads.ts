import { ApiError } from '$lib/api/client';
import type { ApiErrorResponse } from '$lib/api/types';

export interface FileDownloadSelector {
  file_ids?: string[];
  selection_id?: string;
  include_file_ids?: string[];
  exclude_file_ids?: string[];
}

export interface FileDownloadTicket {
  id: string;
  url: string;
}

export async function createFileDownload(
  csrfToken: string,
  selector: FileDownloadSelector,
  signal?: AbortSignal
): Promise<FileDownloadTicket> {
  const response = await fetch('/api/v1/file-downloads', {
    method: 'POST',
    credentials: 'same-origin',
    signal,
    headers: {
      'Content-Type': 'application/json',
      'X-Gooru-CSRF': csrfToken
    },
    body: JSON.stringify(selector)
  });
  const payload = await parseJSON<FileDownloadTicket | ApiErrorResponse>(response);
  if (!response.ok || !payload || !('url' in payload)) {
    const errorPayload = payload as ApiErrorResponse | undefined;
    throw new ApiError(
      response.status,
      errorPayload?.error.code ?? 'http_error',
      errorPayload?.error.message ?? `Request failed with HTTP ${response.status}`
    );
  }
  return payload;
}

async function parseJSON<T>(response: Response): Promise<T | undefined> {
  try {
    return await response.json() as T;
  } catch {
    return undefined;
  }
}
