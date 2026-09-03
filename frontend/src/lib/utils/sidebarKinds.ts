export const sidebarKindFilters = [
  { key: 'photo', label: 'Photos', icon: 'photo', query: 'type:photo' },
  { key: 'video', label: 'Videos', icon: 'video', query: 'type:video' },
  { key: 'gif', label: 'GIFs', icon: 'gif', query: 'type:gif' }
] as const;

const kindTerms = new Set([...sidebarKindFilters.map((kind) => kind.query), 'ext:cbz']);

function terms(query: string) {
  return query.trim().split(/\s+/).filter(Boolean);
}

export function isSidebarKindFilter(term: string) {
  return kindTerms.has(term.toLowerCase() as (typeof sidebarKindFilters)[number]['query'] | 'ext:cbz');
}

export function queryWithoutSidebarKind(query: string) {
  return terms(query).filter((term) => !isSidebarKindFilter(term)).join(' ');
}

export function sidebarKindActive(query: string, filter: string) {
  const normalized = filter.toLowerCase();
  return terms(query).some((term) => term.toLowerCase() === normalized);
}

export function replaceSidebarKind(query: string, filter: string) {
  const base = queryWithoutSidebarKind(query);
  if (sidebarKindActive(query, filter)) return base;
  return [base, filter].filter(Boolean).join(' ');
}

export function appendSidebarKind(query: string, filter: string) {
  return [queryWithoutSidebarKind(query), filter].filter(Boolean).join(' ');
}
