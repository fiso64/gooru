<script lang="ts">
  import type { Snippet } from 'svelte';
  import SearchBar from './SearchBar.svelte';
  import ShellFilterSidebar from './ShellFilterSidebar.svelte';
  import type { MetaTagDefinition } from '$lib/api/types';
  import type { ShellCommonTag, ShellSavedSearch, ShellTagLike } from './shellModel';

  let {
    username,
    route,
    jobsActiveCount,
    jobsDrawerOpen,
    kindCounts,
    comicCount,
    comicAvailable,
    savedSearches,
    commonTags,
    commonTagsCollapsed,
    draggedSavedSearchID,
    savedSearchReorderBusy,
    savedSearchReorderError,
    suggestions,
    metaTags,
    tags,
    search,
    onRoute,
    onOpenLibrary,
    onSavedSearch,
    onCreateSavedSearch,
    onUpdateSavedSearch,
    onDeleteSavedSearch,
    onSearchDraft,
    onSearchCommit,
    onJobs,
    onToggleKind,
    onToggleCommonTags,
    onCommonTag,
    onOpenShortcuts,
    onSavedSearchDragStart,
    onSavedSearchDragPreview,
    onSavedSearchDragEnd,
    onSavedSearchDrop,
    children
  } = $props<{
    username: string;
    route: string;
    jobsActiveCount: number;
    jobsDrawerOpen: boolean;
    kindCounts: Array<{ value: string; count: number }>;
    comicCount: number;
    comicAvailable: boolean;
    savedSearches: ShellSavedSearch[];
    commonTags: ShellCommonTag[];
    commonTagsCollapsed: boolean;
    draggedSavedSearchID: string;
    savedSearchReorderBusy: boolean;
    savedSearchReorderError: string;
    suggestions: Array<{ name: string; count?: number }>;
    metaTags: MetaTagDefinition[];
    tags: ShellTagLike[];
    search: string;
    onRoute: (route: string) => void;
    onOpenLibrary: () => void;
    onSavedSearch: (query: string, name: string) => void;
    onCreateSavedSearch: () => void;
    onUpdateSavedSearch: (id: string, name: string, query: string) => void;
    onDeleteSavedSearch: (id: string, name: string) => void;
    onSearchDraft: (value: string) => void;
    onSearchCommit: (value: string) => void;
    onJobs: () => void;
    onToggleKind: (filter: string) => void;
    onToggleCommonTags: () => void;
    onCommonTag: (tag: string) => void;
    onOpenShortcuts: () => void;
    onSavedSearchDragStart: (event: DragEvent, id: string) => void;
    onSavedSearchDragPreview: (event: DragEvent, id: string) => void;
    onSavedSearchDragEnd: () => void;
    onSavedSearchDrop: (event: DragEvent, id: string) => void | Promise<void>;
    children: Snippet;
  }>();

  const tabs = [
    { route: 'library', label: 'Library' },
    { route: 'tags', label: 'Tags' },
    { route: 'upload', label: 'Upload' },
    { route: 'jobs', label: 'Jobs' },
    { route: 'settings', label: 'Settings' }
  ] as const;
</script>

<div class="app-shell booru-app-shell">
  <header class="booru-header">
    <div class="booru-brand-row">
      <button class="booru-brand" type="button" onclick={onOpenLibrary} aria-label="Gooru library">
        <span class="booru-brand-mark" aria-hidden="true"></span>
        <span>Gooru</span>
      </button>
      <div class="booru-account-actions">
        <button
          class="booru-link-button booru-jobs-button"
          type="button"
          title="Jobs drawer"
          aria-label="Jobs drawer"
          aria-expanded={jobsDrawerOpen}
          aria-controls="jobs-drawer"
          onclick={onJobs}
        >
          Jobs{#if jobsActiveCount > 0} ({jobsActiveCount}){/if}
        </button>
        <button class="booru-link-button" type="button" title={username} onclick={() => onRoute('settings')}>{username}</button>
      </div>
    </div>

    <nav class="booru-main-nav" aria-label="Primary navigation">
      {#each tabs as tab}
        <button
          data-shell-shortcut
          class:current={route === tab.route}
          class="booru-nav-tab"
          type="button"
          aria-current={route === tab.route ? 'page' : undefined}
          onclick={() => tab.route === 'library' ? onOpenLibrary() : onRoute(tab.route)}
        >
          {tab.label}
        </button>
      {/each}
    </nav>

    <nav class="booru-subnav" aria-label="Library utilities">
      <button class:current={route === 'library'} class="booru-subnav-item" type="button" onclick={onOpenLibrary}>Listing</button>
      <button class="booru-subnav-item" type="button" onclick={onCreateSavedSearch}>Save search</button>
      <button class="booru-subnav-item" type="button" onclick={onOpenShortcuts}>Shortcuts</button>
    </nav>
  </header>

  <aside class="sidebar booru-sidebar">
    <section class="booru-search-section" aria-label="Search">
      <h2>Search</h2>
      <form onsubmit={(event) => event.preventDefault()}>
        <SearchBar
          value={search}
          {suggestions}
          {metaTags}
          {tags}
          onDraftInput={onSearchDraft}
          onCommit={onSearchCommit}
          presentation="text"
          placeholder=""
          showShortcutHint={false}
          enableSlashShortcut={false}
        />
      </form>
    </section>

    <ShellFilterSidebar
      {route} {search} {kindCounts} {comicCount} {comicAvailable} {savedSearches} {commonTags} {commonTagsCollapsed}
      {draggedSavedSearchID} {savedSearchReorderBusy} {savedSearchReorderError} onToggleKind={onToggleKind}
      {onCreateSavedSearch} {onUpdateSavedSearch} {onDeleteSavedSearch} {onSavedSearch}
      onSavedSearchDragStart={onSavedSearchDragStart} onSavedSearchDragPreview={onSavedSearchDragPreview}
      onSavedSearchDragEnd={onSavedSearchDragEnd} onSavedSearchDrop={onSavedSearchDrop}
      onToggleCommonTags={onToggleCommonTags} onCommonTag={onCommonTag}
    />
  </aside>

  {@render children()}
</div>
