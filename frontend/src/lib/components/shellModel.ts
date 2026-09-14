import type { MetaTagDefinition } from '$lib/api/types';

export type ShellTagLike = {
  name?: string;
  tag?: string;
  namespace?: string;
  value?: string;
  count?: number;
};

export type ShellSavedSearch = {
  id: string;
  name: string;
  query: string;
};

export type ShellCommonTag = {
  tag: string;
  count: number;
};

export type ShellNavigationRoute = 'library' | 'tags' | 'upload' | 'jobs' | 'settings';

export const shellPrimaryNavigationItems = [
  { route: 'library', label: 'Library', icon: 'library' },
  { route: 'tags', label: 'Tags', icon: 'tags' },
  { route: 'upload', label: 'Upload', icon: 'upload' },
  { route: 'jobs', label: 'Jobs', icon: 'jobs' }
] as const;

export const shellSettingsNavigationItem = { route: 'settings', label: 'Settings', icon: 'settings' } as const;
export const shellTopNavigationItems = [...shellPrimaryNavigationItems, shellSettingsNavigationItem] as const;

export type ShellLayoutModel = {
  username: string;
  route: string;
  libraryCount: number;
  tagCount: number;
  jobsActiveCount: number;
  jobsDrawerOpen: boolean;
  kindCounts: Array<{ value: string; count: number }>;
  comicCount: number;
  comicAvailable: boolean;
  savedSearches: ShellSavedSearch[];
  commonTags: ShellCommonTag[];
  commonTagsCollapsed: boolean;
  draggedSavedSearchID: string;
  savedSearchReorderBusy: boolean;
  savedSearchReorderError: string;
  suggestions: Array<{ name: string; count?: number }>;
  metaTags: MetaTagDefinition[];
  tags: ShellTagLike[];
  search: string;
};

export type ShellLayoutActions = {
  onNavigate: (route: ShellNavigationRoute) => void;
  onSavedSearch: (query: string, name: string) => void;
  onCreateSavedSearch: () => void;
  onUpdateSavedSearch: (id: string, name: string, query: string) => void;
  onDeleteSavedSearch: (id: string, name: string) => void;
  onSearchDraft: (value: string) => void;
  onSearchCommit: (value: string) => void;
  onJobs: () => void;
  onToggleKind: (filter: string) => void;
  onToggleCommonTags: () => void;
  onCommonTag: (tag: string) => void;
  onOpenShortcuts: () => void;
  onSavedSearchDragStart: (event: DragEvent, id: string) => void;
  onSavedSearchDragPreview: (event: DragEvent, id: string) => void;
  onSavedSearchDragEnd: () => void;
  onSavedSearchDrop: (event: DragEvent, id: string) => void | Promise<void>;
};

export type ShellFilterModel = Pick<ShellLayoutModel,
  | 'route'
  | 'search'
  | 'kindCounts'
  | 'comicCount'
  | 'comicAvailable'
  | 'savedSearches'
  | 'commonTags'
  | 'commonTagsCollapsed'
  | 'draggedSavedSearchID'
  | 'savedSearchReorderBusy'
  | 'savedSearchReorderError'
>;

export type ShellFilterActions = Pick<ShellLayoutActions,
  | 'onToggleKind'
  | 'onCreateSavedSearch'
  | 'onUpdateSavedSearch'
  | 'onDeleteSavedSearch'
  | 'onSavedSearch'
  | 'onSavedSearchDragStart'
  | 'onSavedSearchDragPreview'
  | 'onSavedSearchDragEnd'
  | 'onSavedSearchDrop'
  | 'onToggleCommonTags'
  | 'onCommonTag'
>;
