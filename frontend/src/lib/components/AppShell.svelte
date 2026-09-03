<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import JobsDrawer from './JobsDrawer.svelte';
  import Logo from './Logo.svelte';
  import SearchBar from './SearchBar.svelte';
  import type { Job } from '$lib/api/types';

  type TagLike = { name?: string; tag?: string; namespace?: string; value?: string; count?: number };

  let {
    username,
    route,
    activeKind,
    libraryCount,
    tagCount,
    jobsActiveCount,
    jobs,
    jobsDrawerOpen,
    kindCounts,
    comicCount,
    comicActive,
    savedSearches,
    suggestions,
    tags,
    search,
    onRoute,
    onKind,
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
    activeKind: string;
    libraryCount: number;
    tagCount: number;
    jobsActiveCount: number;
    jobs: Job[];
    jobsDrawerOpen: boolean;
    kindCounts: Array<{ value: string; count: number }>;
    comicCount: number;
    comicActive: boolean;
    savedSearches: Array<{ id: string; name: string; query: string }>;
    suggestions: Array<{ name: string; count?: number }>;
    tags: TagLike[];
    search: string;
    onRoute: (route: string) => void;
    onKind: (kind: string) => void;
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

  const kinds = [
    { key: 'photo', label: 'Photos', icon: 'photo' },
    { key: 'video', label: 'Videos', icon: 'video' },
    { key: 'gif', label: 'GIFs', icon: 'gif' }
  ];

  const commonTags = $derived(normalizeCommonTags(tags).slice(0, 20));

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

  function openLibrary() {
    if (route === 'library' && !activeKind) onSearchCommit('');
    onRoute('library');
    onKind('');
  }

  function openCommonTag(tag: string) {
    onRoute('library');
    onKind('');
    onSearchCommit(tag);
  }

  function toggleComics() {
    const terms = search.trim().split(/\s+/).filter((term: string) => term && term.toLowerCase() !== 'ext:cbz');
    if (!comicActive) terms.push('ext:cbz');
    onRoute('library');
    onKind('');
    onSearchCommit(terms.join(' '));
  }
</script>

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
      <button class:active={route === 'library' && !activeKind} class="sidebar-item" type="button" onclick={openLibrary}>
        <Icon name="library" size={16} active={route === 'library' && !activeKind} />
        <span>Library</span>
        <span class="count">{libraryCount.toLocaleString()}</span>
      </button>
      <button class:active={route === 'tags'} class="sidebar-item" type="button" onclick={() => onRoute('tags')}>
        <Icon name="tags" size={16} active={route === 'tags'} />
        <span>Tags</span>
        <span class="count">{tagCount.toLocaleString()}</span>
      </button>
      <button class:active={route === 'upload'} class="sidebar-item" type="button" onclick={() => onRoute('upload')}>
        <Icon name="upload" size={16} active={route === 'upload'} />
        <span>Upload</span>
      </button>
      <button class:active={route === 'jobs'} class="sidebar-item" type="button" onclick={() => onRoute('jobs')}>
        <Icon name="jobs" size={16} active={route === 'jobs'} />
        <span>Jobs</span>
        {#if jobsActiveCount}<span class="count">{jobsActiveCount}</span>{/if}
      </button>
    </div>

    <div class="sidebar-section">
      <div class="sidebar-section-head">Kinds</div>
      {#each kinds as kind}
        <button class:active={route === 'library' && activeKind === kind.key} class="sidebar-item" type="button" onclick={() => { onRoute('library'); onKind(activeKind === kind.key ? '' : kind.key); }}>
          <Icon name={kind.icon} size={16} active={route === 'library' && activeKind === kind.key} />
          <span>{kind.label}</span>
          <span class="count">{kindCount(kind.key).toLocaleString()}</span>
        </button>
      {/each}
      {#if comicCount > 0}
        <button class:active={route === 'library' && comicActive} class="sidebar-item" type="button" onclick={toggleComics}>
          <Icon name="bookmark" size={16} active={route === 'library' && comicActive} />
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
        <div class="sidebar-section-head">Common tags</div>
        {#each commonTags as item}
          <button class="sidebar-item" type="button" onclick={() => openCommonTag(item.tag)}>
            <Icon name="tags" size={14} />
            <span class="truncate">{item.tag}</span>
            <span class="count">{item.count.toLocaleString()}</span>
          </button>
        {/each}
      </div>
    {/if}

    <div class="sidebar-section bottom">
      <button class:active={route === 'settings'} class="sidebar-item" type="button" onclick={() => onRoute('settings')}>
        <Icon name="settings" size={16} />
        <span>Settings</span>
      </button>
      <button class:active={route === 'shortcuts'} class="sidebar-item" type="button" onclick={() => onRoute('shortcuts')}>
        <Icon name="keyboard" size={16} />
        <span>Shortcuts</span>
      </button>
    </div>
  </aside>

  {@render children()}
</div>
