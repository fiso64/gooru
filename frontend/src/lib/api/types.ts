import type { components } from './openapi';

export type MediaUrls = components['schemas']['MediaURLs'];
export type MediaMetadata = components['schemas']['MediaMetadata'];
export type FileItem = components['schemas']['File'];
export type FileListResponse = components['schemas']['FileListResponse'];
export type TagItem = components['schemas']['Tag'];
export type SuggestionsResponse = components['schemas']['SuggestionsResponse'];
export type TagListResponse = components['schemas']['TagListResponse'];
export type NamespacesResponse = components['schemas']['NamespacesResponse'];
export type SavedSearch = components['schemas']['SavedSearch'];
export type SavedSearchesResponse = components['schemas']['SavedSearchesResponse'];
export type TagMutationOperation = components['schemas']['TagMutationResponse']['operation'];
export type TagMutationResponse = components['schemas']['TagMutationResponse'];
export type Job = components['schemas']['Job'];
export type JobListResponse = components['schemas']['JobListResponse'];
export type UploadImportResponse = components['schemas']['UploadImportResponse'];
export type UploadTargetsResponse = components['schemas']['UploadTargetsResponse'];
export type AuthUser = components['schemas']['User'];
export type AuthMeResponse = components['schemas']['AuthMeResponse'];
export type ApiErrorResponse = components['schemas']['ErrorResponse'];

export type SavedSearchRequest = Omit<components['schemas']['SavedSearchRequest'], 'sort' | 'order'> &
  Partial<Pick<components['schemas']['SavedSearchRequest'], 'sort' | 'order'>>;

export type TagMutationRequest = Omit<components['schemas']['TagMutationRequest'], 'verbose'> &
  Partial<Pick<components['schemas']['TagMutationRequest'], 'verbose'>>;
