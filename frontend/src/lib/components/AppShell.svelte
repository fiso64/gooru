<script lang="ts">
  import { onMount } from 'svelte';
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import JobsDrawer from './JobsDrawer.svelte';
  import Logo from './Logo.svelte';
  import SearchBar from './SearchBar.svelte';
  import ShortcutsView from './ShortcutsView.svelte';
  import type { Job, MetaTagDefinition } from '$lib/api/types';
  import { readBrowserPreference, writeBrowserPreference } from '$lib/utils/browserStorage';
  import { hasCommandModifier, isEditableShortcutTarget } from '$lib/utils/keyboard';
  import { replaceSidebarKind, sidebarKindActive, sidebarKindFilters } from '$lib/utils/sidebarKinds';

  type TagLike = { name?: string; tag?: string; namespace?: string; value?: string; count?: number };

  const commonTagsCollapsedKey = 'common-tags.collapsed';
  const isBoolean = (value: unknown): value is boolean => typeof value === 'boolean';

  let {
    username,
    route,
    libraryCount,
    tagCount,
    jobsActiveCount,
    jobs,
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
    jobsDrawerOpen: boolean;
    kindCounts: Array<{ value: string; count: number }>;
    comicCount: number;
    comicAvailable: boolean;
    savedSearches: Array<{ id: string; name: string; query: string }>;
    suggestions: Array<{ name: string; count?: number }>;
    metaTags: MetaTagDefinition[];
    tags: TagLike[];
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

  const kinds = sidebarKindFilters;

  let commonTagsCollapsed = $state(false);
  let shortcutsOpen = $state(false);
  let shortcutsReturnRoute = $state('library');
  const commonTags = $derived(normalizeCommonTags(tags).slice(0, 20));

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

  function kindCount(kind: string) {
    return kindCounts.find((item: { value: string; count: number }) => item.value === kind)?.count ?? 0;
  }

  function normalizeCommonTags(items: TagLike[]) {
    return items
      .map((item) => ({
        tag: item.name ?? item.tag ?? (item.namespace ? `${item.namespace}:${item.value ?? ''}` : (item.value ?? '')),
        count: item.count ?? 0
      }))
      .filter((item) => item.tag)
      .sort((a, b) => b.count - a.count || a.tag.localeCompare(b.tag));
  }

  function commonTagRankPercent(index: number) {
    if (commonTags.length <= 1) return 100;
    return Math.round((1 - index / (commonTags.length - 1)) * 100);
  }

  function toggleCommonTags() {
    commonTagsCollapsed = !commonTagsCollapsed;
    writeBrowserPreference(commonTagsCollapsedKey, commonTagsCollapsed);
  }

  function openLibrary() {
    if (route === 'library') onSearchCommit('');
    onRoute('library');
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
    if (event.defaultPrevented || hasCommandModifier(event) || isEditableShortcutTarget(event.target)) return;
    const shortcutsKey = event.key === '?' || (event.code === 'Slash' && event.shiftKey);
    if (shortcutsKey) {
      event.preventDefault();
      event.stopPropagation();
      openShortcuts();
      return;
    }
    if (shortcutsOpen) {
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        closeShortcuts();
      }
      return;
    }
    if (event.key.toLowerCase() === 'b' && route === 'library' && !document.querySelector('[role="dialog"]')) {
      event.preventDefault();
      event.stopPropagation();
      onCreateSavedSearch();
      return;
    }
    if (!/^[1-9]$/.test(event.key) || document.querySelector('.lightbox[role="dialog"]')) return;

    const index = Number(event.key) - 1;
    const items = Array.from(document.querySelectorAll<HTMLButtonElement>('.sidebar [data-sidebar-shortcut]'))
      .filter((button) => !button.disabled && button.getClientRects().length > 0);
    const target = items[index];
    if (!target) return;
    event.preventDefault();
    target.click();
  }
</script>

<svelte:window onkeydowncapture={handleShellKeydown} />

<div class="app-shell">
  <header class="topbar">
    <button class="topbar-brand" type="button" onclick={() => onRoute('library')} aria-label="Gooru library">
      <span class="topbar-brand-mark"><Logo size={17} /></span>
    </button>
    <form class="topbar-search" onsubmit={(event) => event.preventDefault()}>
      <div class="topbar-search-inner">
        <SearchBar
          value={search}
          {suggestions}
          {metaTags}
          {tags}
          onDraftInput={onSearchDraft}
          onCommit={onSearchCommit}
        />
      </div>
    </form>
    <div class="topbar-right">
      <button
        class="g-btn g-btn-ghost g-btn-sm g-btn-icon"
        type="button"
        title="Jobs"
        aria-label="Jobs"
        aria-expanded={jobsDrawerOpen}
        aria-controls="jobs-drawer"
        onclick={onJobs}
      >
        <Icon name="jobs" size={16} />
        {#if jobsActiveCount > 0}<span class="topbar-badge">{jobsActiveCount}</span>{/if}
      </button>
      <button class="g-btn g-btn-ghost g-btn-sm" type="button" title={username} onclick={() => onRoute('settings')}>
        <Icon name="user" size={14} />
        <span>{username}</span>
      </button>
    </div>
  </header>

  {#if jobsDrawerOpen}
    <div id="jobs-drawer"><JobsDrawer {jobs} onClose={onCloseJobs} onCancel={onCancelJob} /></div>
  {/if}

  <aside class="sidebar">
    <div class="sidebar-section">
      <button data-sidebar-shortcut class:active={route === 'library'} class="sidebar-item" type="button" onclick={openLibrary}>
        <Icon name="library" size={16} active={route === 'library'} />
        <span>Library</span>
        <span class="count">{libraryCount.toLocaleString()}</span>
      </button>
      <button data-sidebar-shortcut class:active={route === 'tags'} class="sidebar-item" type="button" onclick={() => onRoute('tags')}>
        <Icon name="tags" size={16} active={route === 'tags'} />
        <span>Tags</span>
        <span class="count">{tagCount.toLocaleString()}</span>
      </button>
      <button data-sidebar-shortcut class:active={route === 'upload'} class="sidebar-item" type="button" onclick={() => onRoute('upload')}>
        <Icon name="upload" size={16} active={route === 'upload'} />
        <span>Upload</span>
      </button>
      <button data-sidebar-shortcut class:active={route === 'jobs'} class="sidebar-item" type="button" onclick={() => onRoute('jobs')}>
        <Icon name="jobs" size={16} active={route === 'jobs'} />
        <span>Jobs</span>
        {#if jobsActiveCount}<span class="count">{jobsActiveCount}</span>{/if}
      </button>
    </div>

    <div class="sidebar-section">
      <div class="sidebar-section-head">Kinds</div>
      {#each kinds as kind}
        <button data-sidebar-shortcut class:active={route === 'library' && sidebarKindActive(search, kind.query)} class="sidebar-item" type="button" onclick={() => toggleKind(kind.query)}>
          <Icon name={kind.icon} size={16} active={route === 'library' && sidebarKindActive(search, kind.query)} />
          <span>{kind.label}</span>
          <span class="count">{kindCount(kind.key).toLocaleString()}</span>
        </button>
      {/each}
      {#if comicAvailable}
        <button data-sidebar-shortcut class:active={route === 'library' && sidebarKindActive(search, 'ext:cbz')} class="sidebar-item" type="button" onclick={() => toggleKind('ext:cbz')}>
          <Icon name="bookmark" size={16} active={route === 'library' && sidebarKindActive(search, 'ext:cbz')} />
          <span>Comics</span>
          <span class="count">{comicCount.toLocaleString()}</span>
        </button>
      {/if}
    </div>

    <div class="sidebar-section saved-searches-section">
      <div class="sidebar-section-head">
        <span>Saved searches</span>
        <button class="sidebar-head-action" type="button" title="Save current search" aria-label="Save current search" onclick={onCreateSavedSearch}>
          <Icon name="plus" size={11} />
        </button>
      </div>
      {#each savedSearches as saved}
        <div class="sidebar-saved-row">
          <button class="sidebar-item" type="button" onclick={() => onSavedSearch(saved.query, saved.name)}>
            <Icon name="bookmark" size={14} />
            <span class="truncate">{saved.name}</span>
          </button>
          <div class="sidebar-saved-actions">
            <button class="sidebar-mini" type="button" title={`Update ${saved.name}`} aria-label={`Update ${saved.name}`} onclick={() => onUpdateSavedSearch(saved.id, saved.name, saved.query)}>
              <Icon name="check" size={11} />
            </button>
            <button class="sidebar-mini" type="button" title={`Delete ${saved.name}`} aria-label={`Delete ${saved.name}`} onclick={() => onDeleteSavedSearch(saved.id, saved.name)}>
              <Icon name="trash" size={11} />
            </button>
          </div>
        </div>
      {:else}
        <div class="sidebar-note">No saved searches yet</div>
      {/each}
    </div>

    {#if commonTags.length}
      <div class="sidebar-section common-tags-section">
        <button
          class="common-tags-toggle"
          type="button"
          title={commonTagsCollapsed ? 'Expand Common tags' : 'Collapse Common tags'}
          aria-expanded={!commonTagsCollapsed}
          aria-controls="common-tags-list"
          aria-label={commonTagsCollapsed ? 'Expand Common tags' : 'Collapse Common tags'}
          onclick={toggleCommonTags}
        >
          <span class="sidebar-section-head common-tags-toggle-content">
            <span>Common tags</span>
            <span aria-hidden="true" class:expanded={!commonTagsCollapsed} class="common-tags-chevron"><Icon name="chev_right" size={12} /></span>
          </span>
        </button>
        <div id="common-tags-list" class:collapsed={commonTagsCollapsed} class="common-tags-list">
          {#each commonTags as item, index}
            <button
              class="sidebar-item common-tag-item"
              style={`--common-tag-rank: ${commonTagRankPercent(index)}%`}
              type="button"
              onclick={() => openCommonTag(item.tag)}
            >
              <Icon name="tags" size={14} />
              <span class="truncate">{item.tag}</span>
              <span class="count">{item.count.toLocaleString()}</span>
            </button>
          {/each}
        </div>
      </div>
    {/if}

    <div class="sidebar-section bottom">
      <button data-sidebar-shortcut class:active={route === 'settings'} class="sidebar-item" type="button" onclick={() => onRoute('settings')}>
        <Icon name="settings" size={16} />
        <span>Settings</span>
      </button>
      <button class="sidebar-item" type="button" onclick={openShortcuts}>
        <Icon name="keyboard" size={16} />
        <span>Shortcuts</span>
      </button>
    </div>
  </aside>

  {@render children()}
</div>

{#if shortcutsOpen}
  <ShortcutsView onClose={closeShortcuts} />
{/if}

<style>
  .common-tags-toggle {
    width: 100%;
    border: 0;
    padding: 0;
    background: transparent;
    color: inherit;
    cursor: pointer;
    text-align: left;
  }

  .common-tags-toggle:focus-visible {
    outline: 1px solid var(--accent-line);
    outline-offset: -1px;
    border-radius: var(--r-2);
  }

  .common-tags-toggle-content {
    width: 100%;
  }

  .common-tags-chevron {
    display: inline-flex;
    flex: 0 0 auto;
    color: var(--text-3);
    transition: color 0.1s ease, transform 0.1s ease;
  }

  .common-tags-toggle:hover .common-tags-chevron {
    color: var(--text-2);
  }

  .common-tags-chevron.expanded {
    transform: rotate(90deg);
  }

  .common-tags-list {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .common-tags-list.collapsed {
    display: none;
  }

  .common-tag-item {
    color: color-mix(in srgb, var(--text-2) var(--common-tag-rank), var(--text-4));
  }

  .common-tag-item:hover {
    color: var(--text);
  }
</style>
