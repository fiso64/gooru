export type AppRoute = 'library' | 'upload' | 'jobs' | 'tags' | 'settings' | 'shortcuts';

const routePaths: Record<AppRoute, string> = {
  library: '/',
  upload: '/upload',
  jobs: '/jobs',
  tags: '/tags',
  settings: '/settings',
  shortcuts: '/shortcuts'
};

const pathRoutes = new Map(Object.entries(routePaths).map(([route, pathname]) => [pathname, route as AppRoute]));

export function pathForAppRoute(route: string): string {
  return routePaths[route as AppRoute] ?? routePaths.library;
}

export function appRouteFromPath(pathname: string): AppRoute {
  const normalized = pathname !== '/' ? pathname.replace(/\/+$/, '') : '/';
  return pathRoutes.get(normalized) ?? 'library';
}
