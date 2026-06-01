export interface MediaUrls {
  thumbnail: string;
  preview: string;
  content: string;
  download: string;
}

export interface MediaMetadata {
  image_width?: number;
  image_height?: number;
  video_width?: number;
  video_height?: number;
  video_duration?: number;
  audio_duration?: number;
  frame_count?: number;
}

export interface FileItem {
  id: string;
  content_id: string;
  name: string;
  path?: string;
  safe_display_path: string;
  size: number;
  modified_time: string;
  media_type: string;
  media_kind: 'photo' | 'video' | 'gif' | 'audio' | 'other';
  metadata: MediaMetadata;
  tags: string[];
  media_urls: MediaUrls;
}

export interface FileListResponse {
  files: FileItem[];
  next_page_token?: string;
  total_count: number;
  library_count: number;
  facets?: {
    kind?: Array<{ value: string; count: number }>;
  };
}

export interface TagItem {
  name: string;
  namespace?: string;
  value?: string;
  count?: number;
}

export interface SuggestionsResponse {
  items: TagItem[];
}

export interface TagListResponse {
  tags: TagItem[];
}

export interface NamespacesResponse {
  items: string[];
}

export interface SavedSearch {
  id: string;
  name: string;
  query: string;
  sort: 'name' | 'modified' | 'size' | 'kind';
  order: 'asc' | 'desc';
  created_at: string;
  updated_at: string;
}

export interface SavedSearchRequest {
  name: string;
  query: string;
  sort?: 'name' | 'modified' | 'size' | 'kind';
  order?: 'asc' | 'desc';
}

export interface SavedSearchesResponse {
  items: SavedSearch[];
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
  submitted_at: string;
  started_at?: string;
  finished_at?: string;
  result?: unknown;
  error?: string;
}

export interface JobListResponse {
  items: Job[];
}

export interface UploadImportResponse {
  files: Array<{
    name: string;
    size: number;
    target_id: string;
    status: 'uploaded' | 'imported' | 'duplicate_existing' | 'duplicate_in_batch' | 'skipped' | 'error';
    error?: string;
  }>;
  affected_count: number;
}

export interface UploadTargetsResponse {
  items: Array<{ id: string; name: string }>;
}

export interface AuthUser {
  id: string;
  username: string;
  role: 'admin';
}

export interface AuthMeResponse {
  user: AuthUser;
  capabilities: {
    upload: boolean;
    tag: boolean;
    delete: boolean;
    admin: boolean;
  };
  csrf_token?: string;
}

export interface ApiErrorResponse {
  error: {
    code: string;
    message: string;
    details?: unknown;
  };
}
