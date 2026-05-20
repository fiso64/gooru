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

export interface ApiErrorResponse {
  error: {
    code: string;
    message: string;
    details?: unknown;
  };
}
