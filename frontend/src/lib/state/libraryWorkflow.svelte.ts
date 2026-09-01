import { writable } from 'svelte/store';
import type { FileItem } from '$lib/api/types';
import type { FileSort, SortOrder } from '$lib/queries/files';

export function createLibraryWorkflow() {
  const searchDraft = writable('');
  const submittedSearch = writable('');
  const suggestionSearch = writable('');
  let route = $state('library');
  let activeKind = $state('');
  let activeSavedSearch = $state('');
  let sort: FileSort = $state('modified');
  let order: SortOrder = $state('desc');
  let selectedIDs = $state(new Set<string>());
  let activeFile = $state<FileItem | null>(null);
  let searchDebounce: ReturnType<typeof setTimeout> | undefined;
  let suggestionDebounce: ReturnType<typeof setTimeout> | undefined;

  function reset() {
    selectedIDs = new Set();
    activeFile = null;
  }

  function submitSearch() {
    submittedSearch.set(get(searchDraft).trim());
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
    submittedSearch.set(query);
    selectedIDs = new Set();
    route = 'library';
  }

  function filterQuery() {
    const parts = [get(submittedSearch).trim()];
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
    submittedSearch.set(query);
    selectedIDs = new Set();
    route = 'library';
  }

  function runSavedSearch(query: string, name: string) {
    activeSavedSearch = name;
    searchDraft.set(query);
    suggestionSearch.set(query);
    submittedSearch.set(query);
    route = 'library';
  }

  function applySuggestion(value: string) {
    const next = [get(searchDraft).trim(), value].filter(Boolean).join(' ');
    searchDraft.set(next);
    suggestionSearch.set(next);
    submittedSearch.set(next);
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
  }

  function closePreview() {
    activeFile = null;
  }

  function movePreview(delta: number, files: FileItem[]) {
    if (!activeFile || !files.length) return;
    const index = files.findIndex((file) => file.id === activeFile?.id);
    activeFile = files[(index + delta + files.length) % files.length] ?? activeFile;
  }

  function handleKeydown(event: KeyboardEvent, files: FileItem[]) {
    if (event.key === 'Escape' && activeFile) closePreview();
    if (activeFile && (event.key === 'ArrowLeft' || event.key === 'k')) movePreview(-1, files);
    if (activeFile && (event.key === 'ArrowRight' || event.key === 'j')) movePreview(1, files);
  }

  function setRoute(next: string) {
    route = next;
  }


  return {
    searchDraft,
    submittedSearch,
    suggestionSearch,
    get route() { return route; },
    set route(value: string) { route = value; },
    get activeKind() { return activeKind; },
    get activeSavedSearch() { return activeSavedSearch; },
    get sort() { return sort; },
    set sort(value: FileSort) { sort = value; },
    get order() { return order; },
    set order(value: SortOrder) { order = value; },
    get selectedIDs() { return selectedIDs; },
    set selectedIDs(value: Set<string>) { selectedIDs = value; },
    get activeFile() { return activeFile; },
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
