import type { components } from './openapi';

export type MediaUrls = components['schemas']['MediaURLs'];
export type MediaMetadata = components['schemas']['MediaMetadata'];
export type FileItem = components['schemas']['File'];
export type FileListResponse = components['schemas']['FileListResponse'];
export type FileRemovalRequest = components['schemas']['FileRemovalRequest'];
export type FileRemovalResponse = components['schemas']['FileRemovalResponse'];
export type ComicPage = components['schemas']['ComicPage'];
export type ComicManifest = components['schemas']['ComicManifest'];
export type TagItem = components['schemas']['Tag'];
export type MetaTagDefinition = components['schemas']['MetaTag'];
export type SuggestionsResponse = components['schemas']['SuggestionsResponse'];
export type TagListResponse = components['schemas']['TagListResponse'];
export type NamespacesResponse = components['schemas']['NamespacesResponse'];
export type SavedSearch = components['schemas']['SavedSearch'];
export type SavedSearchesResponse = components['schemas']['SavedSearchesResponse'];
export type TagMutationOperation = components['schemas']['TagMutationResponse']['operation'];
export type TagMutationResponse = components['schemas']['TagMutationResponse'];
export type JobStatus = 'pending' | 'running' | 'completed' | 'failed' | 'canceled';
export type JobOutcome = 'success' | 'partial_success' | 'error';
export interface Job {
  id: string;
  type: string;
  status: JobStatus;
  outcome?: JobOutcome;
  affected_count?: number;
  failed_count?: number;
  stage?: 'receiving' | 'importing';
  progress?: number;
  progress_total?: number;
  progress_completed?: number;
  progress_completed_prefix?: number;
  progress_failed?: number;
  submitted_at: string;
  started_at?: string;
  finished_at?: string;
  result?: unknown;
  error?: string;
}
type GeneratedUploadImportResponse = components['schemas']['UploadImportResponse'];
export type UploadImportResponse = Omit<GeneratedUploadImportResponse, 'files'> & {
  files: Array<GeneratedUploadImportResponse['files'][number] & { id?: string }>;
};
export type UploadTargetsResponse = components['schemas']['UploadTargetsResponse'];
export type AuthUser = components['schemas']['User'];
export type AuthMeResponse = components['schemas']['AuthMeResponse'];
export type ApiErrorResponse = components['schemas']['ErrorResponse'];

export type SavedSearchRequest = Omit<components['schemas']['SavedSearchRequest'], 'sort' | 'order'> &
  Partial<Pick<components['schemas']['SavedSearchRequest'], 'sort' | 'order'>>;

export type TagMutationRequest = Omit<components['schemas']['TagMutationRequest'], 'verbose'> &
  Partial<Pick<components['schemas']['TagMutationRequest'], 'verbose'>>;
