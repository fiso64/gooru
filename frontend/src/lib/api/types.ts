export interface MediaUrls {
  thumbnail: string;
  preview: string;
  content: string;
}

export interface FileItem {
  id: string;
  content_id: string;
  name: string;
  path?: string;
  size: number;
  modified_time: string;
  media_type: string;
  media_kind: 'image' | 'video' | 'audio' | 'other';
  tags: string[];
  media_urls: MediaUrls;
}

export interface FileListResponse {
  files: FileItem[];
  next_page_token?: string;
}

export type TagMutationOperation = 'add' | 'set' | 'remove';

export interface TagMutationRequest {
  file_ids?: string[];
  query?: string;
  tags: string[];
  verbose?: boolean;
}

export interface TagMutationResponse {
  operation: TagMutationOperation;
  selector: {
    file_ids?: string[];
    query?: string;
  };
  matched_files?: number;
  affected_count: number;
}

export interface Job {
  id: string;
  type: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'canceled';
  progress?: number;
  result?: unknown;
  error?: string;
}

export interface UploadImportResponse {
  files: Array<{ name: string; size: number }>;
  affected_count: number;
}

export interface ApiErrorResponse {
  error: {
    code: string;
    message: string;
    details?: unknown;
  };
}
