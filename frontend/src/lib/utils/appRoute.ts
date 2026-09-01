import type { FileSort, SortOrder } from '$lib/queries/files';

export type AppRoute = 'library' | 'upload' | 'jobs' | 'tags' | 'settings' | 'shortcuts';

export interface LibraryURLState {
  query: string;
  kind: string;
  sort: FileSort;
  order: SortOrder;
}

export const defaultLibraryURLState: LibraryURLState = {
  query: '',
  kind: '',
  sort: 'modified',
  order: 'desc'
};

const routePaths: Record<AppRoute, string> = {
  library: '/',
  upload: '/upload',
  jobs: '/jobs',
  tags: '/tags',
  settings: '/settings',
  shortcuts: '/shortcuts'
};

const pathRoutes = new Map(Object.entries(routePaths).map(([route, pathname]) => [pathname, route as AppRoute]));
const fileSorts = new Set<FileSort>(['modified', 'name', 'size', 'kind']);

export function pathForAppRoute(route: string): string {
  return routePaths[route as AppRoute] ?? routePaths.library;
}

export function appRouteFromPath(pathname: string): AppRoute {
  const normalized = pathname !== '/' ? pathname.replace(/\/+$/, '') : '/';
  return pathRoutes.get(normalized) ?? 'library';
}

export function libraryURLStateFromSearch(search: string): LibraryURLState {
  const params = new URLSearchParams(search);
  const sort = params.get('sort') as FileSort | null;
  const order = params.get('order');
  return {
    query: params.get('q')?.trim() ?? '',
    kind: params.get('type')?.trim() ?? '',
    sort: sort && fileSorts.has(sort) ? sort : defaultLibraryURLState.sort,
    order: order === 'asc' || order === 'desc' ? order : defaultLibraryURLState.order
  };
}

export function searchForLibraryURLState(state: LibraryURLState): string {
  const params = new URLSearchParams();
  const query = state.query.trim();
  const kind = state.kind.trim();
  if (query) params.set('q', query);
  if (kind) params.set('type', kind);
  if (state.sort !== defaultLibraryURLState.sort) params.set('sort', state.sort);
  if (state.order !== defaultLibraryURLState.order) params.set('order', state.order);
  const encoded = params.toString();
  return encoded ? `?${encoded}` : '';
}
