import { browser } from '$app/environment';
import { writable } from 'svelte/store';
import { ApiClient } from '$lib/api/client';
import { useOpaqueURLState } from '$lib/api/privacy';
import { authState } from '$lib/stores/auth';
import type { FileItem } from '$lib/api/types';
import type { FileSort, SortOrder } from '$lib/queries/files';
import {
  appRouteFromPath,
  defaultLibraryURLState,
  libraryURLStateFromSearch,
  pathForAppRoute,
  searchForLibraryURLState,
  type AppRoute
} from '$lib/utils/appRoute';
import { isEditableShortcutTarget } from '$lib/utils/keyboard';
import { replaceSidebarKind } from '$lib/utils/sidebarKinds';
import { previewNeighbor } from '$lib/utils/viewerNavigation';
import { clearViewerPreloadCache } from '$lib/utils/viewerPreload';
import {
  applySelectionMembership,
  applySelectionSnapshot,
  emptySelection,
  failSelectionSnapshot,
  selectAllMatching,
  selectionActive,
  selectionCount,
  selectionError,
  selectionHas,
  selectionPending,
  selectionRequestID,
  selectionSnapshotID,
  selectionUnknownIDs,
  setSelectionRange,
  toggleSelection,
  type LibrarySelection
} from '$lib/state/selection';

export function createLibraryWorkflow(
  initialRoute: AppRoute = browser ? appRouteFromPath(window.location.pathname) : 'library',
  initialPaginationEnabled = false
) {
  const opaqueURLState = browser && useOpaqueURLState();
  const initialOpaqueToken = opaqueURLState && initialRoute === 'library'
    ? (new URLSearchParams(window.location.search).get('state')?.trim() ?? '')
    : '';
  const initialLibraryState = browser && initialRoute === 'library' && !initialOpaqueToken
    ? libraryURLStateFromSearch(window.location.search)
    : defaultLibraryURLState;
  const initialQuery = initialLibraryState.kind
    ? replaceSidebarKind(initialLibraryState.query, `type:${initialLibraryState.kind}`)
    : initialLibraryState.query;
  const searchDraft = writable(initialQuery);
  const submittedSearch = writable(initialQuery);
  const suggestionSearch = writable(initialQuery);
  let submittedQuery = $state(initialQuery);
  let route: AppRoute = $state(initialRoute);
  let activeKind = $state('');
  let activeSavedSearch = $state('');
  let sort: FileSort = $state(initialLibraryState.sort);
  let order: SortOrder = $state(initialLibraryState.order);
  let page = $state(initialLibraryState.page);
  let paginationEnabled = $state(initialPaginationEnabled);
  let selection: LibrarySelection = $state(emptySelection());
  let selectionAnchorID = $state('');
  let selectionGeneration = 0;
  let activeFile = $state<FileItem | null>(null);
  let pendingPreviewID = $state(initialLibraryState.fileID);
  let searchDebounce: ReturnType<typeof setTimeout> | undefined;
  let historyGeneration = 0;
  let restoreGeneration = 0;
  let restoredStateKey = opaqueURLState && !window.location.search ? stateKey(initialLibraryState) : '';
  let initialOpaqueRestorePending = $state(Boolean(initialOpaqueToken));

  function setSubmittedSearch(value: string) {
    submittedQuery = value;
    submittedSearch.set(value);
  }

  function normalizePage(value: number) {
    return Number.isSafeInteger(value) && value > 0 ? value : 1;
  }

  function stateKey(state: typeof defaultLibraryURLState) { return JSON.stringify(state); }

  function currentHistoryState() {
    return {
      query: submittedQuery,
      kind: '',
      sort,
      order,
      fileID: pendingPreviewID,
      page: paginationEnabled ? page : 1
    };
  }

  function applyRestoredLibraryState(nextLibraryState: typeof defaultLibraryURLState) {
    restoredStateKey = stateKey({ ...nextLibraryState, page: paginationEnabled ? normalizePage(nextLibraryState.page) : 1 });
    const restoredQuery = nextLibraryState.kind ? replaceSidebarKind(nextLibraryState.query, `type:${nextLibraryState.kind}`) : nextLibraryState.query;
    activeKind = '';
    activeSavedSearch = '';
    sort = nextLibraryState.sort;
    order = nextLibraryState.order;
    page = normalizePage(nextLibraryState.page);
    pendingPreviewID = nextLibraryState.fileID;
    searchDraft.set(restoredQuery);
    suggestionSearch.set(restoredQuery);
    setSubmittedSearch(restoredQuery);
    activeFile = null;
    selection = emptySelection();
    selectionAnchorID = '';
  }

  async function resolveOpaqueState(token: string, signal?: AbortSignal) {
    return new ApiClient(get(authState).csrfToken).resolveURLState(token, signal);
  }

  $effect(() => {
    if (!browser || !initialOpaqueRestorePending || !initialOpaqueToken) return;
    const controller = new AbortController();
    resolveOpaqueState(initialOpaqueToken, controller.signal).then((state) => {
      if (controller.signal.aborted) return;
      applyRestoredLibraryState(state);
      initialOpaqueRestorePending = false;
    }).catch((error) => {
      if (controller.signal.aborted) return;
      console.warn('Unable to restore protected library URL state', error);
      restoredStateKey = stateKey(defaultLibraryURLState);
      window.history.replaceState(null, '', pathForAppRoute('library'));
      initialOpaqueRestorePending = false;
    });
    return () => controller.abort();
  });

  // Keep top-level navigation, durable library controls, the selected page, and
  // the active preview in one browser-history policy. Protected mode seals the
  // entire library state server-side before it enters the address bar.
  $effect(() => {
    if (!browser || initialOpaqueRestorePending) return;
    const pathname = pathForAppRoute(route);
    const generation = ++historyGeneration;
    if (route !== 'library') {
      const currentURL = `${window.location.pathname}${window.location.search}`;
      if (currentURL !== pathname) window.history.pushState(null, '', pathname);
      return;
    }
    const state = currentHistoryState();
    const key = stateKey(state);
    if (restoredStateKey === key) { restoredStateKey = ''; return; }
    if (!opaqueURLState) {
      const nextURL = `${pathname}${searchForLibraryURLState(state)}`;
      const currentURL = `${window.location.pathname}${window.location.search}`;
      if (currentURL !== nextURL) window.history.pushState(null, '', nextURL);
      return;
    }
    if (key === stateKey(defaultLibraryURLState)) {
      if (`${window.location.pathname}${window.location.search}` !== pathname) window.history.pushState(null, '', pathname);
      return;
    }
    const replaceLegacy = !new URLSearchParams(window.location.search).has('state') && window.location.search !== '';
    new ApiClient(get(authState).csrfToken).createURLState(state).then((token) => {
      if (generation !== historyGeneration) return;
      const nextURL = `${pathname}?state=${encodeURIComponent(token)}`;
      if (`${window.location.pathname}${window.location.search}` === nextURL) return;
      if (replaceLegacy) window.history.replaceState(null, '', nextURL);
      else window.history.pushState(null, '', nextURL);
    }).catch((error) => console.warn('Unable to protect library URL state', error));
  });

  $effect(() => {
    if (!browser) return;
    const restoreRoute = async () => {
      const generation = ++restoreGeneration;
      const nextRoute = appRouteFromPath(window.location.pathname);
      route = nextRoute;
      if (nextRoute !== 'library') {
        applyRestoredLibraryState(defaultLibraryURLState);
        return;
      }
      const token = opaqueURLState ? (new URLSearchParams(window.location.search).get('state')?.trim() ?? '') : '';
      try {
        const nextLibraryState = token ? await resolveOpaqueState(token) : libraryURLStateFromSearch(window.location.search);
        if (generation !== restoreGeneration) return;
        applyRestoredLibraryState(nextLibraryState);
      } catch (error) {
        if (generation !== restoreGeneration) return;
        console.warn('Unable to restore protected library history state', error);
        applyRestoredLibraryState(defaultLibraryURLState);
        window.history.replaceState(null, '', pathForAppRoute('library'));
      }
    };
    window.addEventListener('popstate', restoreRoute);
    return () => window.removeEventListener('popstate', restoreRoute);
  });

  // AuthenticatedApp only instantiates this workflow after session resolution,
  // so a direct preview URL can safely hydrate the file from the canonical API.
  // Normal grid opens already carry the complete File object and skip this fetch.
  $effect(() => {
    if (!browser || route !== 'library') return;
    const id = pendingPreviewID;
    if (!id || activeFile?.id === id) return;
    const controller = new AbortController();
    new ApiClient()
      .getFile(id, controller.signal)
      .then((file) => {
        if (!controller.signal.aborted && pendingPreviewID === id) activeFile = file;
      })
      .catch((error) => {
        if (controller.signal.aborted) return;
        if (pendingPreviewID === id) {
          activeFile = null;
          pendingPreviewID = '';
        }
        console.warn('Unable to restore media preview from URL', error);
      });
    return () => controller.abort();
  });

  function resetPage() {
    page = 1;
  }

  function reset() {
    selection = emptySelection();
    selectionAnchorID = '';
    activeFile = null;
    clearViewerPreloadCache();
  }

  function submitSearch() {
    setSubmittedSearch(get(searchDraft).trim());
    resetPage();
    selection = emptySelection();
    selectionAnchorID = '';
    route = 'library';
  }

  function setSearch(value: string) {
    searchDraft.set(value);
    suggestionSearch.set(value.trim());
    if (searchDebounce) clearTimeout(searchDebounce);
    searchDebounce = setTimeout(submitSearch, 280);
  }

  function setSearchDraft(value: string) {
    suggestionSearch.set(value.trim());
  }

  function commitSearch(value: string) {
    if (searchDebounce) clearTimeout(searchDebounce);
    const query = value.trim();
    activeKind = '';
    searchDraft.set(query);
    suggestionSearch.set(query);
    setSubmittedSearch(query);
    resetPage();
    selection = emptySelection();
    selectionAnchorID = '';
    route = 'library';
  }

  function filterQuery() {
    return submittedQuery.trim();
  }

  function setKind(kind: string) {
    activeKind = '';
    commitSearch(kind ? replaceSidebarKind(get(searchDraft), `type:${kind}`) : get(searchDraft));
  }

  function runTagSearch(query: string) {
    activeKind = '';
    searchDraft.set(query);
    suggestionSearch.set(query);
    setSubmittedSearch(query);
    resetPage();
    selection = emptySelection();
    selectionAnchorID = '';
    activeFile = null;
    pendingPreviewID = '';
    route = 'library';
  }

  function runSavedSearch(query: string, name: string) {
    activeKind = '';
    activeSavedSearch = name;
    searchDraft.set(query);
    suggestionSearch.set(query);
    setSubmittedSearch(query);
    resetPage();
    selection = emptySelection();
    selectionAnchorID = '';
    route = 'library';
  }

  function applySuggestion(value: string) {
    const next = [get(searchDraft).trim(), value].filter(Boolean).join(' ');
    searchDraft.set(next);
    suggestionSearch.set(next);
    setSubmittedSearch(next);
    resetPage();
    selection = emptySelection();
    selectionAnchorID = '';
    route = 'library';
  }

  function toggleSelect(file: FileItem, files: FileItem[] = [], range = false) {
    const targetSelected = selectionHas(selection, file.id);
    const anchorIndex = selectionAnchorID ? files.findIndex((candidate) => candidate.id === selectionAnchorID) : -1;
    const targetIndex = files.findIndex((candidate) => candidate.id === file.id);
    if (range && anchorIndex >= 0 && targetIndex >= 0) {
      const start = Math.min(anchorIndex, targetIndex);
      const end = Math.max(anchorIndex, targetIndex);
      selection = setSelectionRange(selection, files.slice(start, end + 1).map((candidate) => candidate.id), !targetSelected);
      if (!targetSelected) selectionAnchorID = file.id;
      else if (!selectionHas(selection, selectionAnchorID)) selectionAnchorID = '';
      return;
    }

    selection = toggleSelection(selection, file.id);
    if (!targetSelected) selectionAnchorID = file.id;
    else if (selectionAnchorID === file.id) selectionAnchorID = '';
  }

  function extendSelection(current: FileItem, target: FileItem, files: FileItem[]) {
    const currentIndex = files.findIndex((candidate) => candidate.id === current.id);
    const targetIndex = files.findIndex((candidate) => candidate.id === target.id);
    if (currentIndex < 0 || targetIndex < 0) return;

    let anchorIndex = selectionAnchorID ? files.findIndex((candidate) => candidate.id === selectionAnchorID) : -1;
    if (anchorIndex < 0) {
      selectionAnchorID = current.id;
      anchorIndex = currentIndex;
    }

    const rangeIDs = (from: number, to: number) => {
      const start = Math.min(from, to);
      const end = Math.max(from, to);
      return files.slice(start, end + 1).map((candidate) => candidate.id);
    };
    selection = setSelectionRange(selection, rangeIDs(anchorIndex, currentIndex), false);
    selection = setSelectionRange(selection, rangeIDs(anchorIndex, targetIndex), true);
  }

  function selectAll(optimisticCount: number, files: FileItem[] = []) {
    const requestID = ++selectionGeneration;
    selection = selectAllMatching(filterQuery(), optimisticCount, files.map((file) => file.id), requestID);
    selectionAnchorID = '';
    return requestID;
  }

  function applySnapshot(requestID: number, snapshotID: string, count: number) {
    selection = applySelectionSnapshot(selection, requestID, snapshotID, count);
  }

  function failSnapshot(requestID: number, error: string) {
    selection = failSelectionSnapshot(selection, requestID, error);
  }

  function applySnapshotMembership(snapshotID: string, candidates: string[], members: string[]) {
    selection = applySelectionMembership(selection, snapshotID, candidates, members);
  }

  function unknownSnapshotIDs(fileIDs: string[]) {
    return selectionUnknownIDs(selection, fileIDs);
  }

  function clearSelection() {
    selection = emptySelection();
    selectionAnchorID = '';
  }

  function isSelected(fileID: string) {
    return selectionHas(selection, fileID);
  }

  function selectedCount() {
    return selectionCount(selection);
  }

  function openPreview(file: FileItem, files: FileItem[] = []) {
    void files;
    clearViewerPreloadCache();
    activeFile = file;
    pendingPreviewID = file.id;
    route = 'library';
  }

  function closePreview() {
    activeFile = null;
    pendingPreviewID = '';
  }

  function movePreview(delta: number, files: FileItem[]) {
    const next = previewNeighbor(activeFile, files, delta);
    if (!next) return;
    // Rapid navigation is latest-wins: stale speculative decodes must not remain queued
    // ahead of the browser's foreground request for the newly requested file.
    clearViewerPreloadCache();
    activeFile = next;
    pendingPreviewID = next.id;
  }

  function handleKeydown(event: KeyboardEvent, files: FileItem[]) {
    if (event.defaultPrevented || isEditableShortcutTarget(event.target)) return;

    if (event.key === 'Escape') {
      if (activeFile) closePreview();
      else if (selectionActive(selection)) clearSelection();
      return;
    }
    if (activeFile && !event.shiftKey && (event.key === 'ArrowLeft' || event.key === 'k')) {
      event.preventDefault();
      movePreview(-1, files);
      return;
    }
    if (activeFile && !event.shiftKey && (event.key === 'ArrowRight' || event.key === 'j')) {
      event.preventDefault();
      movePreview(1, files);
    }
  }

  function setRoute(next: string) {
    route = appRouteFromPath(pathForAppRoute(next));
    if (route !== 'library') {
      activeFile = null;
      pendingPreviewID = '';
      selection = emptySelection();
      selectionAnchorID = '';
    }
  }

  return {
    searchDraft,
    submittedSearch,
    suggestionSearch,
    get route() { return route; },
    set route(value: AppRoute) { route = value; },
    get activeKind() { return activeKind; },
    get activeSavedSearch() { return activeSavedSearch; },
    get sort() { return sort; },
    set sort(value: FileSort) { sort = value; resetPage(); },
    get order() { return order; },
    set order(value: SortOrder) { order = value; resetPage(); },
    get page() { return page; },
    set page(value: number) { page = normalizePage(value); },
    get selection() { return selection; },
    get selectionPending() { return selectionPending(selection); },
    get selectionError() { return selectionError(selection); },
    get selectionSnapshotID() { return selectionSnapshotID(selection); },
    get selectionRequestID() { return selectionRequestID(selection); },
    get activeFile() { return activeFile; },
    get pendingPreviewID() { return pendingPreviewID; },
    setPaginationEnabled(value: boolean) { paginationEnabled = value; },
    reset,
    submitSearch,
    setSearch,
    setSearchDraft,
    commitSearch,
    filterQuery,
    setKind,
    runTagSearch,
    runSavedSearch,
    applySuggestion,
    toggleSelect,
    extendSelection,
    selectAll,
    applySnapshot,
    failSnapshot,
    applySnapshotMembership,
    unknownSnapshotIDs,
    clearSelection,
    isSelected,
    selectedCount,
    openPreview,
    closePreview,
    movePreview,
    handleKeydown,
    setRoute
  };
}

function get<T>(store: { subscribe: (run: (value: T) => void) => () => void }): T {
  let value: T;
  const unsubscribe = store.subscribe((next) => (value = next));
  unsubscribe();
  return value!;
}
