import { browser } from '$app/environment';
import { writable } from 'svelte/store';
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
  let selectedIDs = $state(new Set<string>());
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
      selectedIDs = new Set();
    };
    window.addEventListener('popstate', restoreRoute);
    return () => window.removeEventListener('popstate', restoreRoute);
  });

  function reset() {
    selectedIDs = new Set();
    activeFile = null;
  }

  function submitSearch() {
    setSubmittedSearch(get(searchDraft).trim());
    selectedIDs = new Set();
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
    selectedIDs = new Set();
    route = 'library';
  }

  function filterQuery() {
    const parts = [submittedQuery.trim()];
    if (activeKind) parts.push(`type:${activeKind}`);
    return parts.filter(Boolean).join(' ');
  }

  function setKind(kind: string) {
    activeKind = kind;
    selectedIDs = new Set();
  }

  function runTagSearch(query: string) {
    activeKind = '';
    searchDraft.set(query);
    suggestionSearch.set(query);
    setSubmittedSearch(query);
    selectedIDs = new Set();
    activeFile = null;
    pendingPreviewID = '';
    route = 'library';
  }

  function runSavedSearch(query: string, name: string) {
    activeSavedSearch = name;
    searchDraft.set(query);
    suggestionSearch.set(query);
    setSubmittedSearch(query);
    route = 'library';
  }

  function applySuggestion(value: string) {
    const next = [get(searchDraft).trim(), value].filter(Boolean).join(' ');
    searchDraft.set(next);
    suggestionSearch.set(next);
    setSubmittedSearch(next);
    route = 'library';
  }

  function toggleSelect(file: FileItem) {
    const next = new Set(selectedIDs);
    if (next.has(file.id)) next.delete(file.id);
    else next.add(file.id);
    selectedIDs = next;
  }

  function selectFiles(files: FileItem[]) {
    selectedIDs = new Set(files.map((file) => file.id));
  }

  function clearSelection() {
    selectedIDs = new Set();
  }

  function openPreview(file: FileItem) {
    activeFile = file;
    pendingPreviewID = file.id;
    route = 'library';
  }

  function resolvePreview(file: FileItem) {
    if (pendingPreviewID !== file.id) return;
    activeFile = file;
  }

  function closePreview() {
    activeFile = null;
    pendingPreviewID = '';
  }

  function movePreview(delta: number, files: FileItem[]) {
    if (!activeFile || !files.length) return;
    const index = files.findIndex((file) => file.id === activeFile?.id);
    const next = files[(index + delta + files.length) % files.length] ?? activeFile;
    activeFile = next;
    pendingPreviewID = next.id;
  }

  function handleKeydown(event: KeyboardEvent, files: FileItem[]) {
    if (event.defaultPrevented || isEditableShortcutTarget(event.target)) return;

    if (event.key === 'Escape') {
      if (activeFile) closePreview();
      else if (selectedIDs.size) clearSelection();
      return;
    }
    if (activeFile && (event.key === 'ArrowLeft' || event.key === 'k')) {
      event.preventDefault();
      movePreview(-1, files);
      return;
    }
    if (activeFile && (event.key === 'ArrowRight' || event.key === 'j')) {
      event.preventDefault();
      movePreview(1, files);
    }
  }

  function setRoute(next: string) {
    route = appRouteFromPath(pathForAppRoute(next));
    if (route !== 'library') {
      activeFile = null;
      pendingPreviewID = '';
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
    get selectedIDs() { return selectedIDs; },
    set selectedIDs(value: Set<string>) { selectedIDs = value; },
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
    selectFiles,
    clearSelection,
    openPreview,
    resolvePreview,
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
