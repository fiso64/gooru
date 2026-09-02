export type ExplicitSelection = {
  mode: 'explicit';
  ids: Set<string>;
};

export type QuerySelection = {
  mode: 'query';
  query: string;
  excludedIDs: Set<string>;
};

export type LibrarySelection = ExplicitSelection | QuerySelection;

export function emptySelection(): LibrarySelection {
  return { mode: 'explicit', ids: new Set() };
}

export function selectAllMatching(query: string): LibrarySelection {
  return { mode: 'query', query: query.trim() || '*', excludedIDs: new Set() };
}

export function selectionHas(selection: LibrarySelection, fileID: string): boolean {
  if (selection.mode === 'query') return !selection.excludedIDs.has(fileID);
  return selection.ids.has(fileID);
}

export function selectionCount(selection: LibrarySelection, totalCount: number): number {
  if (selection.mode === 'query') return Math.max(0, totalCount - selection.excludedIDs.size);
  return selection.ids.size;
}

export function selectionActive(selection: LibrarySelection): boolean {
  return selection.mode === 'query' || selection.ids.size > 0;
}

export function toggleSelection(selection: LibrarySelection, fileID: string): LibrarySelection {
  if (selection.mode === 'query') {
    const excludedIDs = new Set(selection.excludedIDs);
    if (excludedIDs.has(fileID)) excludedIDs.delete(fileID);
    else excludedIDs.add(fileID);
    return { ...selection, excludedIDs };
  }

  const ids = new Set(selection.ids);
  if (ids.has(fileID)) ids.delete(fileID);
  else ids.add(fileID);
  return { mode: 'explicit', ids };
}

export function selectionRequest(selection: LibrarySelection):
  | { file_ids: string[] }
  | { query: string; exclude_file_ids?: string[] } {
  if (selection.mode === 'query') {
    const excluded = [...selection.excludedIDs];
    return {
      query: selection.query,
      ...(excluded.length ? { exclude_file_ids: excluded } : {})
    };
  }
  return { file_ids: [...selection.ids] };
}
