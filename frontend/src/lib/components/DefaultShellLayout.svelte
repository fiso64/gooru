<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import Logo from './Logo.svelte';
  import SearchBar from './SearchBar.svelte';
  import ShellFilterSidebar from './ShellFilterSidebar.svelte';
  import type { Job, MetaTagDefinition } from '$lib/api/types';
  import type { ShellCommonTag, ShellSavedSearch, ShellTagLike } from './shellModel';

  let {
    username,
    route,
    libraryCount,
    tagCount,
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
    libraryCount: number;
    tagCount: number;
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
</script>

<div class="app-shell default-app-shell">
  <header class="topbar">
    <button class="topbar-brand" type="button" onclick={onOpenLibrary} aria-label="Gooru library">
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

  <aside class="sidebar">
    <div class="sidebar-section">
      <button data-shell-shortcut class:active={route === 'library'} class="sidebar-item" type="button" onclick={onOpenLibrary}>
        <Icon name="library" size={16} active={route === 'library'} />
        <span>Library</span>
        <span class="count">{libraryCount.toLocaleString()}</span>
      </button>
      <button data-shell-shortcut class:active={route === 'tags'} class="sidebar-item" type="button" onclick={() => onRoute('tags')}>
        <Icon name="tags" size={16} active={route === 'tags'} />
        <span>Tags</span>
        <span class="count">{tagCount.toLocaleString()}</span>
      </button>
      <button data-shell-shortcut class:active={route === 'upload'} class="sidebar-item" type="button" onclick={() => onRoute('upload')}>
        <Icon name="upload" size={16} active={route === 'upload'} />
        <span>Upload</span>
      </button>
      <button data-shell-shortcut class:active={route === 'jobs'} class="sidebar-item" type="button" onclick={() => onRoute('jobs')}>
        <Icon name="jobs" size={16} active={route === 'jobs'} />
        <span>Jobs</span>
        {#if jobsActiveCount}<span class="count">{jobsActiveCount}</span>{/if}
      </button>
    </div>

    <ShellFilterSidebar
      {route}
      {search}
      {kindCounts}
      {comicCount}
      {comicAvailable}
      {savedSearches}
      {commonTags}
      {commonTagsCollapsed}
      {draggedSavedSearchID}
      {savedSearchReorderBusy}
      {savedSearchReorderError}
      onToggleKind={onToggleKind}
      {onCreateSavedSearch}
      {onUpdateSavedSearch}
      {onDeleteSavedSearch}
      {onSavedSearch}
      onSavedSearchDragStart={onSavedSearchDragStart}
      onSavedSearchDragPreview={onSavedSearchDragPreview}
      onSavedSearchDragEnd={onSavedSearchDragEnd}
      onSavedSearchDrop={onSavedSearchDrop}
      onToggleCommonTags={onToggleCommonTags}
      onCommonTag={onCommonTag}
    />

    <div class="sidebar-section bottom">
      <button data-shell-shortcut class:active={route === 'settings'} class="sidebar-item" type="button" onclick={() => onRoute('settings')}>
        <Icon name="settings" size={16} />
        <span>Settings</span>
      </button>
      <button class="sidebar-item" type="button" onclick={onOpenShortcuts}>
        <Icon name="keyboard" size={16} />
        <span>Shortcuts</span>
      </button>
    </div>
  </aside>

  {@render children()}
</div>
