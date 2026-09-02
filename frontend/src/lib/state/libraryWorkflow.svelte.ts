import { browser } from '$app/environment';
import { writable } from 'svelte/store';
import { ApiClient } from '$lib/api/client';
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
import { previewNeighbor } from '$lib/utils/viewerNavigation';
import { clearViewerPreloadCache, preloadViewerMedia } from '$lib/utils/viewerPreload';
import {
  emptySelection,
  selectAllMatching,
  selectionActive,
  selectionCount,
  selectionHas,
  setSelectionRange,
  toggleSelection,
  type LibrarySelection
} from '$lib/state/selection';

export function createLibraryWorkflow(initialRoute: AppRoute = browser ? appRouteFromPath(window.location.pathname) : 'library') {
  const initialLibraryState = browser && initialRoute === 'library'
    ? libraryURLStateFromSearch(window.location.search)
    : defaultLibraryURLState;
  const searchDraft = writable(initialLibraryState.query);
  const submittedSearch = writable(initialLibraryState.query);
  const suggestionSearch = writable(initialLibraryState.query);
  let submittedQuery = $state(initialLibraryState.query);
  let route: AppRoute = $state(initialRoute);
  let activeKind = $state(initialLibraryState.kind);
  let activeSavedSearch = $state('');
  let sort: FileSort = $state(initialLibraryState.sort);
  let order: SortOrder = $state(initialLibraryState.order);
  let selection: LibrarySelection = $state(emptySelection());
  let selectionAnchorID = $state('');
  let activeFile = $state<FileItem | null>(null);
  let pendingPreviewID = $state(initialLibraryState.fileID);
  let searchDebounce: ReturnType<typeof setTimeout> | undefined;
  let suggestionDebounce: ReturnType<typeof setTimeout> | undefined;

  function setSubmittedSearch(value: string) {
    submittedQuery = value;
    submittedSearch.set(value);
  }

  // Keep top-level navigation, durable library controls, and the active preview
  // in one browser-history policy. Components remain unaware of the History API,
  // and a preview URL can be restored independently of the currently loaded page.
  $effect(() => {
    if (!browser) return;
    const pathname = pathForAppRoute(route);
    const search = route === 'library'
      ? searchForLibraryURLState({ query: submittedQuery, kind: activeKind, sort, order, fileID: pendingPreviewID })
      : '';
    const nextURL = `${pathname}${search}`;
    const currentURL = `${window.location.pathname}${window.location.search}`;
    if (currentURL !== nextURL) window.history.pushState(null, '', nextURL);
  });

  $effect(() => {
    if (!browser) return;
    const restoreRoute = () => {
      const nextRoute = appRouteFromPath(window.location.pathname);
      const nextLibraryState = nextRoute === 'library'
        ? libraryURLStateFromSearch(window.location.search)
        : defaultLibraryURLState;
      route = nextRoute;
      activeKind = nextLibraryState.kind;
      activeSavedSearch = '';
      sort = nextLibraryState.sort;
      order = nextLibraryState.order;
      pendingPreviewID = nextLibraryState.fileID;
      searchDraft.set(nextLibraryState.query);
      suggestionSearch.set(nextLibraryState.query);
      setSubmittedSearch(nextLibraryState.query);
      activeFile = null;
      selection = emptySelection();
      selectionAnchorID = '';
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

  function reset() {
    selection = emptySelection();
    selectionAnchorID = '';
    activeFile = null;
    clearViewerPreloadCache();
  }

  function submitSearch() {
    setSubmittedSearch(get(searchDraft).trim());
    selection = emptySelection();
    selectionAnchorID = '';
    route = 'library';
  }

  function setSearch(value: string) {
    searchDraft.set(value);
    if (searchDebounce) clearTimeout(searchDebounce);
    if (suggestionDebounce) clearTimeout(suggestionDebounce);
    searchDebounce = setTimeout(submitSearch, 280);
    suggestionDebounce = setTimeout(() => suggestionSearch.set(value.trim()), 160);
  }

  function setSearchDraft(value: string) {
    if (suggestionDebounce) clearTimeout(suggestionDebounce);
    suggestionDebounce = setTimeout(() => suggestionSearch.set(value.trim()), 160);
  }

  function commitSearch(value: string) {
    if (searchDebounce) clearTimeout(searchDebounce);
    if (suggestionDebounce) clearTimeout(suggestionDebounce);
    const query = value.trim();
    searchDraft.set(query);
    suggestionSearch.set(query);
    setSubmittedSearch(query);
    selection = emptySelection();
    selectionAnchorID = '';
    route = 'library';
  }

  function filterQuery() {
    const parts = [submittedQuery.trim()];
    if (activeKind) parts.push(`type:${activeKind}`);
    return parts.filter(Boolean).join(' ');
  }

  function setKind(kind: string) {
    activeKind = kind;
    selection = emptySelection();
    selectionAnchorID = '';
  }

  function runTagSearch(query: string) {
    activeKind = '';
    searchDraft.set(query);
    suggestionSearch.set(query);
    setSubmittedSearch(query);
    selection = emptySelection();
    selectionAnchorID = '';
    activeFile = null;
    pendingPreviewID = '';
    route = 'library';
  }

  function runSavedSearch(query: string, name: string) {
    activeSavedSearch = name;
    searchDraft.set(query);
    suggestionSearch.set(query);
    setSubmittedSearch(query);
    selection = emptySelection();
    selectionAnchorID = '';
    route = 'library';
  }

  function applySuggestion(value: string) {
    const next = [get(searchDraft).trim(), value].filter(Boolean).join(' ');
    searchDraft.set(next);
    suggestionSearch.set(next);
    setSubmittedSearch(next);
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

  function selectAll() {
    selection = selectAllMatching(filterQuery());
    selectionAnchorID = '';
  }

  function clearSelection() {
    selection = emptySelection();
    selectionAnchorID = '';
  }

  function isSelected(fileID: string) {
    return selectionHas(selection, fileID);
  }

  function selectedCount(totalCount: number) {
    return selectionCount(selection, totalCount);
  }

  function primePreviewNeighbor(file: FileItem, files: FileItem[]) {
    const next = previewNeighbor(file, files, 1);
    if (next) void preloadViewerMedia(next).catch(() => undefined);
  }

  function openPreview(file: FileItem, files: FileItem[] = []) {
    activeFile = file;
    pendingPreviewID = file.id;
    route = 'library';
    primePreviewNeighbor(file, files);
  }

  function closePreview() {
    activeFile = null;
    pendingPreviewID = '';
  }

  function movePreview(delta: number, files: FileItem[]) {
    const next = previewNeighbor(activeFile, files, delta);
    if (!next) return;
    void preloadViewerMedia(next).catch(() => undefined);
    activeFile = next;
    pendingPreviewID = next.id;
    primePreviewNeighbor(next, files);
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
    set sort(value: FileSort) { sort = value; },
    get order() { return order; },
    set order(value: SortOrder) { order = value; },
    get selection() { return selection; },
    get activeFile() { return activeFile; },
    get pendingPreviewID() { return pendingPreviewID; },
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
    selectAll,
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
