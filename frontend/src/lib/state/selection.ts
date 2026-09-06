export type ExplicitSelection = {
  mode: 'explicit';
  ids: Set<string>;
};

export type SnapshotSelection = {
  mode: 'snapshot';
  requestID: number;
  query: string;
  snapshotID: string;
  snapshotCount: number | null;
  optimisticCount: number;
  optimisticIDs: Set<string>;
  knownMembers: Set<string>;
  knownNonMembers: Set<string>;
  includedIDs: Set<string>;
  excludedIDs: Set<string>;
  error: string;
};

export type LibrarySelection = ExplicitSelection | SnapshotSelection;

export type SelectionRequest =
  | { file_ids: string[] }
  | { selection_id: string; include_file_ids?: string[]; exclude_file_ids?: string[] };

export function emptySelection(): LibrarySelection {
  return { mode: 'explicit', ids: new Set() };
}

export function selectAllMatching(query: string, optimisticCount: number, visibleIDs: string[], requestID: number): LibrarySelection {
  return {
    mode: 'snapshot',
    requestID,
    query: query.trim() || '*',
    snapshotID: '',
    snapshotCount: null,
    optimisticCount: Math.max(0, optimisticCount),
    optimisticIDs: new Set(visibleIDs),
    knownMembers: new Set(),
    knownNonMembers: new Set(),
    includedIDs: new Set(),
    excludedIDs: new Set(),
    error: ''
  };
}

export function selectionHas(selection: LibrarySelection, fileID: string): boolean {
  if (selection.mode === 'explicit') return selection.ids.has(fileID);
  if (selection.excludedIDs.has(fileID)) return false;
  if (selection.includedIDs.has(fileID)) return true;
  if (selection.knownMembers.has(fileID)) return true;
  if (selection.knownNonMembers.has(fileID)) return false;
  return selection.optimisticIDs.has(fileID);
}

export function selectionCount(selection: LibrarySelection): number {
  if (selection.mode === 'explicit') return selection.ids.size;
  const base = selection.snapshotCount ?? selection.optimisticCount;
  return Math.max(0, base - selection.excludedIDs.size + selection.includedIDs.size);
}

export function selectionActive(selection: LibrarySelection): boolean {
  return selection.mode === 'snapshot' || selection.ids.size > 0;
}

export function selectionPending(selection: LibrarySelection): boolean {
  return selection.mode === 'snapshot' && !selection.snapshotID && !selection.error;
}

export function selectionError(selection: LibrarySelection): string {
  return selection.mode === 'snapshot' ? selection.error : '';
}

export function selectionSnapshotID(selection: LibrarySelection): string {
  return selection.mode === 'snapshot' ? selection.snapshotID : '';
}

export function selectionRequestID(selection: LibrarySelection): number {
  return selection.mode === 'snapshot' ? selection.requestID : 0;
}

export function applySelectionSnapshot(selection: LibrarySelection, requestID: number, snapshotID: string, count: number): LibrarySelection {
  if (selection.mode !== 'snapshot' || selection.requestID !== requestID) return selection;
  return {
    ...selection,
    snapshotID,
    snapshotCount: Math.max(0, count),
    error: ''
  };
}

export function failSelectionSnapshot(selection: LibrarySelection, requestID: number, error: string): LibrarySelection {
  if (selection.mode !== 'snapshot' || selection.requestID !== requestID) return selection;
  return { ...selection, snapshotID: '', error };
}

export function selectionUnknownIDs(selection: LibrarySelection, fileIDs: string[]): string[] {
  if (selection.mode !== 'snapshot' || !selection.snapshotID) return [];
  return fileIDs.filter((fileID) =>
    !selection.knownMembers.has(fileID) &&
    !selection.knownNonMembers.has(fileID)
  );
}

export function applySelectionMembership(
  selection: LibrarySelection,
  snapshotID: string,
  candidates: string[],
  members: string[]
): LibrarySelection {
  if (selection.mode !== 'snapshot' || selection.snapshotID !== snapshotID) return selection;
  const memberSet = new Set(members);
  const knownMembers = new Set(selection.knownMembers);
  const knownNonMembers = new Set(selection.knownNonMembers);
  const optimisticIDs = new Set(selection.optimisticIDs);
  const includedIDs = new Set(selection.includedIDs);
  const excludedIDs = new Set(selection.excludedIDs);

  for (const fileID of candidates) {
    optimisticIDs.delete(fileID);
    if (memberSet.has(fileID)) {
      knownMembers.add(fileID);
      knownNonMembers.delete(fileID);
      includedIDs.delete(fileID);
    } else {
      knownNonMembers.add(fileID);
      knownMembers.delete(fileID);
      excludedIDs.delete(fileID);
    }
  }

  return {
    ...selection,
    optimisticIDs,
    knownMembers,
    knownNonMembers,
    includedIDs,
    excludedIDs
  };
}

export function toggleSelection(selection: LibrarySelection, fileID: string): LibrarySelection {
  if (selection.mode === 'snapshot') {
    const selected = selectionHas(selection, fileID);
    const includedIDs = new Set(selection.includedIDs);
    const excludedIDs = new Set(selection.excludedIDs);

    if (selected) {
      includedIDs.delete(fileID);
      excludedIDs.add(fileID);
    } else if (selection.knownMembers.has(fileID) || selection.optimisticIDs.has(fileID)) {
      excludedIDs.delete(fileID);
    } else {
      excludedIDs.delete(fileID);
      includedIDs.add(fileID);
    }
    return { ...selection, includedIDs, excludedIDs };
  }

  const ids = new Set(selection.ids);
  if (ids.has(fileID)) ids.delete(fileID);
  else ids.add(fileID);
  return { mode: 'explicit', ids };
}

export function setSelectionRange(selection: LibrarySelection, fileIDs: string[], selected: boolean): LibrarySelection {
  if (selection.mode === 'snapshot') {
    let next: LibrarySelection = selection;
    for (const fileID of fileIDs) {
      if (selectionHas(next, fileID) !== selected) next = toggleSelection(next, fileID);
    }
    return next;
  }

  const ids = new Set(selection.ids);
  for (const fileID of fileIDs) {
    if (selected) ids.add(fileID);
    else ids.delete(fileID);
  }
  return { mode: 'explicit', ids };
}

export function selectionRequest(selection: LibrarySelection): SelectionRequest {
  if (selection.mode === 'snapshot') {
    if (!selection.snapshotID) throw new Error(selection.error || 'Selection snapshot is still loading.');
    const included = [...selection.includedIDs];
    const excluded = [...selection.excludedIDs];
    return {
      selection_id: selection.snapshotID,
      ...(included.length ? { include_file_ids: included } : {}),
      ...(excluded.length ? { exclude_file_ids: excluded } : {})
    };
  }
  return { file_ids: [...selection.ids] };
}
