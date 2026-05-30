export interface MediaUrls {
  thumbnail: string;
  preview: string;
  content: string;
}

export interface MediaMetadata {
  image_width?: number;
  image_height?: number;
  video_width?: number;
  video_height?: number;
  video_duration?: number;
  audio_duration?: number;
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
  metadata: MediaMetadata;
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
  submitted_at: string;
  started_at?: string;
  finished_at?: string;
  result?: unknown;
  error?: string;
}

export interface UploadImportResponse {
  files: Array<{ name: string; size: number }>;
  affected_count: number;
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
