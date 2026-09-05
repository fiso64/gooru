export type PageControl = number | 'ellipsis';

export function paginationWindow(currentPage: number, pageCount: number, radius = 2): PageControl[] {
  const total = Math.max(1, Math.floor(pageCount));
  const current = Math.min(total, Math.max(1, Math.floor(currentPage)));
  const nearby = Math.max(0, Math.floor(radius));
  const pages = new Set<number>([1, total]);
  for (let page = Math.max(1, current - nearby); page <= Math.min(total, current + nearby); page += 1) {
    pages.add(page);
  }
  const sorted = [...pages].sort((a, b) => a - b);
  const controls: PageControl[] = [];
  let previous = 0;
  for (const page of sorted) {
    if (previous && page - previous > 1) controls.push('ellipsis');
    controls.push(page);
    previous = page;
  }
  return controls;
}

export function offsetPageToken(pageIndex: number, pageSize: number) {
  const offset = Math.max(0, Math.floor(pageIndex)) * Math.max(1, Math.floor(pageSize));
  if (!offset) return '';
  return btoa(`offset:${offset}`).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}
