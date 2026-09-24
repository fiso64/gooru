<script lang="ts">
  import { onMount } from 'svelte';
  import type { Snippet } from 'svelte';
  import BooruShellLayout from './BooruShellLayout.svelte';
  import DefaultShellLayout from './DefaultShellLayout.svelte';
  import JobsDrawer from './JobsDrawer.svelte';
  import ShortcutsView from './ShortcutsView.svelte';
  import type { ShellLayoutActions, ShellLayoutModel, ShellNavigationRoute, ShellSavedSearch, ShellTagLike } from './shellModel';
  import { ApiClient } from '$lib/api/client';
  import type { Job, MetaTagDefinition } from '$lib/api/types';
  import { authState } from '$lib/stores/auth';
  import { runtimeConfig } from '$lib/stores/runtimeConfig';
  import { readBrowserPreference, writeBrowserPreference } from '$lib/utils/browserStorage';
  import { matchesShortcut, matchesShortcutCode, matchesShortcutModifiers, isEditableShortcutTarget } from '$lib/utils/keyboard';
  import { hasBlockingModal } from '$lib/utils/modal';
  import { replaceSidebarKind } from '$lib/utils/sidebarKinds';

  const commonTagsCollapsedKey = 'common-tags.collapsed';
  const isBoolean = (value: unknown): value is boolean => typeof value === 'boolean';

  let {
    username,
    route,
    libraryCount,
    tagCount,
    jobsActiveCount,
    jobs,
    jobsTotalCount,
    jobsDrawerOpen,
    kindCounts,
    comicCount,
    comicAvailable,
    savedSearches,
    suggestions,
    metaTags,
    tags,
    search,
    onRoute,
    onSavedSearch,
    onCreateSavedSearch,
    onUpdateSavedSearch,
    onDeleteSavedSearch,
    onSearchDraft,
    onSearchCommit,
    onJobs,
    onCloseJobs,
    onCancelJob,
    children
  } = $props<{
    username: string;
    route: string;
    libraryCount: number;
    tagCount: number;
    jobsActiveCount: number;
    jobs: Job[];
    jobsTotalCount: number;
    jobsDrawerOpen: boolean;
    kindCounts: Array<{ value: string; count: number }>;
    comicCount: number;
    comicAvailable: boolean;
    savedSearches: ShellSavedSearch[];
    suggestions: Array<{ name: string; count?: number }>;
    metaTags: MetaTagDefinition[];
    tags: ShellTagLike[];
    search: string;
    onRoute: (route: string) => void;
    onSavedSearch: (query: string, name: string) => void;
    onCreateSavedSearch: () => void;
    onUpdateSavedSearch: (id: string, name: string, query: string) => void;
    onDeleteSavedSearch: (id: string, name: string) => void;
    onSearchDraft: (value: string) => void;
    onSearchCommit: (value: string) => void;
    onJobs: () => void;
    onCloseJobs: () => void;
    onCancelJob: (job: Job) => void;
    children: Snippet;
  }>();

  let commonTagsCollapsed = $state(false);
  let shortcutsOpen = $state(false);
  let shortcutsReturnRoute = $state('library');
  let draggedSavedSearchID = $state('');
  let savedSearchDragStartOrder = $state<string[]>([]);
  let savedSearchOrder = $state<string[]>([]);
  let savedSearchMembership = $state('');
  let savedSearchReorderBusy = $state(false);
  let savedSearchReorderError = $state('');

  const commonTags = $derived(normalizeCommonTags(tags).slice(0, 20));
  const orderedSavedSearches = $derived.by(() => {
    const byID = new Map(savedSearches.map((item: ShellSavedSearch) => [item.id, item]));
    const ordered = savedSearchOrder.map((id) => byID.get(id)).filter((item): item is ShellSavedSearch => Boolean(item));
    const seen = new Set(ordered.map((item: ShellSavedSearch) => item.id));
    return [...ordered, ...savedSearches.filter((item: ShellSavedSearch) => !seen.has(item.id))];
  });

  onMount(() => {
    commonTagsCollapsed = readBrowserPreference(commonTagsCollapsedKey, false, isBoolean);
  });

  $effect(() => {
    if (route !== 'shortcuts') {
      shortcutsReturnRoute = route;
      return;
    }
    shortcutsOpen = true;
    queueMicrotask(() => onRoute(shortcutsReturnRoute));
  });

  $effect(() => {
    const ids = savedSearches.map((item: ShellSavedSearch) => item.id);
    const membership = [...ids].sort().join('\u0000');
    if (membership !== savedSearchMembership) {
      savedSearchMembership = membership;
      savedSearchOrder = ids;
    }
  });

  function normalizeCommonTags(items: ShellTagLike[]) {
    return items
      .map((item) => ({
        tag: item.name ?? item.tag ?? (item.namespace ? `${item.namespace}:${item.value ?? ''}` : (item.value ?? '')),
        count: item.count ?? 0
      }))
      .filter((item) => item.tag)
      .sort((a, b) => b.count - a.count || a.tag.localeCompare(b.tag));
  }

  function toggleCommonTags() {
    commonTagsCollapsed = !commonTagsCollapsed;
    writeBrowserPreference(commonTagsCollapsedKey, commonTagsCollapsed);
  }

  function startSavedSearchDrag(event: DragEvent, id: string) {
    if (savedSearchReorderBusy) {
      event.preventDefault();
      return;
    }
    draggedSavedSearchID = id;
    savedSearchDragStartOrder = orderedSavedSearches.map((item: ShellSavedSearch) => item.id);
    savedSearchReorderError = '';
    event.dataTransfer?.setData('text/plain', id);
    if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move';
  }

  function previewSavedSearchDrag(event: DragEvent, targetID: string) {
    const sourceID = draggedSavedSearchID || event.dataTransfer?.getData('text/plain') || '';
    if (!sourceID || savedSearchReorderBusy) return;
    event.preventDefault();
    if (sourceID === targetID) return;

    const current = orderedSavedSearches.map((item: ShellSavedSearch) => item.id);
    const from = current.indexOf(sourceID);
    const target = current.indexOf(targetID);
    if (from < 0 || target < 0) return;

    const targetRow = event.currentTarget instanceof HTMLElement ? event.currentTarget : null;
    const bounds = targetRow?.getBoundingClientRect();
    const insertAfter = bounds ? event.clientY >= bounds.top + bounds.height / 2 : from < target;
    const next = current.filter((id) => id !== sourceID);
    const targetAfterRemoval = next.indexOf(targetID);
    if (targetAfterRemoval < 0) return;
    next.splice(targetAfterRemoval + (insertAfter ? 1 : 0), 0, sourceID);
    if (next.every((id, index) => id === current[index])) return;
    savedSearchOrder = next;
  }

  function endSavedSearchDrag() {
    draggedSavedSearchID = '';
    if (!savedSearchDragStartOrder.length) return;
    savedSearchOrder = savedSearchDragStartOrder;
    savedSearchDragStartOrder = [];
  }

  async function dropSavedSearch(event: DragEvent, targetID: string) {
    event.preventDefault();
    const sourceID = draggedSavedSearchID || event.dataTransfer?.getData('text/plain') || '';
    if (!sourceID || savedSearchReorderBusy) return;

    previewSavedSearchDrag(event, targetID);
    const previous = savedSearchDragStartOrder.length ? [...savedSearchDragStartOrder] : orderedSavedSearches.map((item: ShellSavedSearch) => item.id);
    const next = orderedSavedSearches.map((item: ShellSavedSearch) => item.id);
    draggedSavedSearchID = '';
    savedSearchDragStartOrder = [];
    if (next.every((id, index) => id === previous[index])) return;

    savedSearchOrder = next;
    savedSearchReorderBusy = true;
    savedSearchReorderError = '';
    try {
      await new ApiClient($authState.csrfToken).reorderSavedSearches(next);
    } catch (error) {
      savedSearchOrder = previous;
      savedSearchReorderError = error instanceof Error ? error.message : 'Could not save saved-search order.';
    } finally {
      savedSearchReorderBusy = false;
    }
  }

  function navigate(destination: ShellNavigationRoute) {
    if (destination === 'library') {
      if (route === 'library') onSearchCommit('');
      onRoute('library');
      return;
    }
    onRoute(destination);
  }

  function openAllJobs() {
    onCloseJobs();
    onRoute('jobs');
  }

  function openCommonTag(tag: string) {
    onSearchCommit(tag);
  }

  function toggleKind(filter: string) {
    onRoute('library');
    onSearchCommit(replaceSidebarKind(search, filter));
  }

  function openShortcuts() {
    shortcutsOpen = true;
  }

  function closeShortcuts() {
    shortcutsOpen = false;
  }

  function handleShellKeydown(event: KeyboardEvent) {
    if (event.defaultPrevented || isEditableShortcutTarget(event.target)) return;
    if (shortcutsOpen) {
      if (matchesShortcut(event, 'Escape')) {
        event.preventDefault();
        event.stopPropagation();
        closeShortcuts();
      }
      return;
    }
    if (hasBlockingModal()) return;
    const shortcutsKey = matchesShortcut(event, '?') || matchesShortcut(event, '?', { shift: true }) || matchesShortcutCode(event, 'Slash', { shift: true });
    if (shortcutsKey) {
      event.preventDefault();
      event.stopPropagation();
      openShortcuts();
      return;
    }
    if (!matchesShortcutModifiers(event)) return;
    if (matchesShortcut(event, 'b') && route === 'library' && !document.querySelector('[role="dialog"]')) {
      event.preventDefault();
      event.stopPropagation();
      onCreateSavedSearch();
      return;
    }
    if (!/^[1-9]$/.test(event.key) || document.querySelector('.lightbox[role="dialog"]')) return;

    const index = Number(event.key) - 1;
    const items = Array.from(document.querySelectorAll<HTMLButtonElement>('[data-shell-shortcut]'))
      .filter((button) => !button.disabled && button.getClientRects().length > 0);
    const target = items[index];
    if (!target) return;
    event.preventDefault();
    target.click();
  }

  const layoutModel = $derived.by((): ShellLayoutModel => ({
    username,
    route,
    libraryCount,
    tagCount,
    jobsActiveCount,
    jobsDrawerOpen,
    kindCounts,
    comicCount,
    comicAvailable,
    savedSearches: orderedSavedSearches,
    commonTags,
    commonTagsCollapsed,
    draggedSavedSearchID,
    savedSearchReorderBusy,
    savedSearchReorderError,
    suggestions,
    metaTags,
    tags,
    search
  }));

  const layoutActions = $derived.by((): ShellLayoutActions => ({
    onNavigate: navigate,
    onSavedSearch,
    onCreateSavedSearch,
    onUpdateSavedSearch,
    onDeleteSavedSearch,
    onSearchDraft,
    onSearchCommit,
    onJobs,
    onToggleKind: toggleKind,
    onToggleCommonTags: toggleCommonTags,
    onCommonTag: openCommonTag,
    onOpenShortcuts: openShortcuts,
    onSavedSearchDragStart: startSavedSearchDrag,
    onSavedSearchDragPreview: previewSavedSearchDrag,
    onSavedSearchDragEnd: endSavedSearchDrag,
    onSavedSearchDrop: dropSavedSearch
  }));
</script>

<svelte:window onkeydowncapture={handleShellKeydown} />

{#if $runtimeConfig.uiTheme === 'booru-style'}
  <BooruShellLayout model={layoutModel} actions={layoutActions} {children} />
{:else}
  <DefaultShellLayout model={layoutModel} actions={layoutActions} {children} />
{/if}

{#if jobsDrawerOpen}
  <div id="jobs-drawer"><JobsDrawer {jobs} totalCount={jobsTotalCount} onViewAll={openAllJobs} onClose={onCloseJobs} onCancel={onCancelJob} /></div>
{/if}

{#if shortcutsOpen}
  <ShortcutsView onClose={closeShortcuts} />
{/if}
