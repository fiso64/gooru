import { browser } from '$app/environment';
import { get, writable } from 'svelte/store';
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
  let selection: LibrarySelection = $state(emptySelection());
  let selectionAnchorID = $state('');
  let activeFile = $state<FileItem | null>(null);
  let pendingPreviewID = $state(initialLibraryState.fileID);
  let searchDebounce: ReturnType<typeof setTimeout> | undefined;
  let suggestionDebounce: ReturnType<typeof setTimeout> | undefined;
  let historyGeneration = 0;
  let restoredStateKey = opaqueURLState && !window.location.search ? stateKey(initialLibraryState) : '';
  let initialOpaqueRestorePending = Boolean(initialOpaqueToken);

  function setSubmittedSearch(value: string) {
    submittedQuery = value;
    submittedSearch.set(value);
  }

  function stateKey(state: typeof defaultLibraryURLState) { return JSON.stringify(state); }

  function applyRestoredLibraryState(nextLibraryState: typeof defaultLibraryURLState) {
    restoredStateKey = stateKey(nextLibraryState);
    const restoredQuery = nextLibraryState.kind ? replaceSidebarKind(nextLibraryState.query, `type:${nextLibraryState.kind}`) : nextLibraryState.query;
    activeKind = '';
    activeSavedSearch = '';
    sort = nextLibraryState.sort;
    order = nextLibraryState.order;
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
    initialOpaqueRestorePending = false;
    const controller = new AbortController();
    resolveOpaqueState(initialOpaqueToken, controller.signal).then(applyRestoredLibraryState).catch((error) => {
      if (controller.signal.aborted) return;
      console.warn('Unable to restore protected library URL state', error);
      restoredStateKey = stateKey(defaultLibraryURLState);
      window.history.replaceState(null, '', pathForAppRoute('library'));
    });
    return () => controller.abort();
  });

  // Keep top-level navigation, durable library controls, and the active preview
  // in one browser-history policy. Protected mode seals library state server-side
  // before it enters the address bar; ordinary mode retains readable URLs.
  $effect(() => {
    if (!browser || initialOpaqueRestorePending) return;
    const pathname = pathForAppRoute(route);
    if (route !== 'library') {
      const currentURL = `${window.location.pathname}${window.location.search}`;
      if (currentURL !== pathname) window.history.pushState(null, '', pathname);
      return;
    }
    const state = { query: submittedQuery, kind: '', sort, order, fileID: pendingPreviewID };
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
    const generation = ++historyGeneration;
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
      const nextRoute = appRouteFromPath(window.location.pathname);
      route = nextRoute;
      if (nextRoute !== 'library') {
        applyRestoredLibraryState(defaultLibraryURLState);
        return;
      }
      const token = opaqueURLState ? (new URLSearchParams(window.location.search).get('state')?.trim() ?? '') : '';
      try {
        const nextLibraryState = token ? await resolveOpaqueState(token) : libraryURLStateFromSearch(window.location.search);
        applyRestoredLibraryState(nextLibraryState);
      } catch (error) {
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
    activeKind = '';
    searchDraft.set(query);
    suggestionSearch.set(query);
    setSubmittedSearch(query);
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
