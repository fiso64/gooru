<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import Logo from './Logo.svelte';

  let {
    username,
    route,
    activeKind,
    libraryCount,
    tagCount,
    jobsActiveCount,
    kindCounts,
    savedSearches,
    suggestions,
    search,
    onRoute,
    onKind,
    onSavedSearch,
    onSuggestion,
    onSearchInput,
    onSearchSubmit,
    onJobs,
    children
  } = $props<{
    username: string;
    route: string;
    activeKind: string;
    libraryCount: number;
    tagCount: number;
    jobsActiveCount: number;
    kindCounts: Array<{ value: string; count: number }>;
    savedSearches: Array<{ id: string; name: string; query: string }>;
    suggestions: Array<{ name: string; count?: number }>;
    search: string;
    onRoute: (route: string) => void;
    onKind: (kind: string) => void;
    onSavedSearch: (query: string, name: string) => void;
    onSuggestion: (value: string) => void;
    onSearchInput: (value: string) => void;
    onSearchSubmit: () => void;
    onJobs: () => void;
    children: Snippet;
  }>();

  const kinds = [
    { key: 'photo', label: 'Photos', icon: 'photo' },
    { key: 'video', label: 'Videos', icon: 'video' },
    { key: 'gif', label: 'GIFs', icon: 'gif' },
    { key: 'other', label: 'Other', icon: 'folder' }
  ];

  function kindCount(kind: string) {
    return kindCounts.find((item: { value: string; count: number }) => item.value === kind)?.count ?? 0;
  }
</script>

<div class="app-shell">
  <header class="topbar">
    <button class="topbar-brand" type="button" onclick={() => onRoute('library')} aria-label="Gooru library">
      <span class="topbar-brand-mark"><Logo size={17} /></span>
    </button>
    <form class="topbar-search" onsubmit={(event) => { event.preventDefault(); onSearchSubmit(); }}>
      <div class="searchbar">
        <span class="searchbar-icon"><Icon name="search" size={16} /></span>
        <input
          class="searchbar-input"
          value={search}
          oninput={(event) => onSearchInput(event.currentTarget.value)}
          placeholder="tag, key:value, @tagged"
          aria-label="Search library"
        />
        {#if search}
          <button class="searchbar-clear" type="button" aria-label="Clear search" onclick={() => { onSearchInput(''); onSearchSubmit(); }}>
            <Icon name="close" size={12} />
          </button>
        {/if}
      </div>
      {#if suggestions.length}
        <div class="search-suggestions" role="listbox" aria-label="Search suggestions">
          <div class="group-head">Suggestions</div>
          {#each suggestions as suggestion}
            <button type="button" role="option" aria-selected="false" onclick={() => onSuggestion(suggestion.name)}>
              <span class="tok">{suggestion.name}</span>
              {#if suggestion.count != null}<span class="count">{suggestion.count.toLocaleString()}</span>{/if}
            </button>
          {/each}
        </div>
      {/if}
    </form>
    <div class="topbar-right">
      <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" title="Jobs" aria-label="Jobs" onclick={onJobs}>
        <Icon name="jobs" size={16} />
        {#if jobsActiveCount > 0}<span class="topbar-badge">{jobsActiveCount}</span>{/if}
      </button>
      <button class="g-btn g-btn-ghost g-btn-sm" type="button" title={username} onclick={() => onRoute('account')}>
        <Icon name="user" size={14} />
        <span>{username}</span>
      </button>
    </div>
  </header>

  <aside class="sidebar">
    <div class="sidebar-section">
      <button class:active={route === 'library' && !activeKind} class="sidebar-item" type="button" onclick={() => { onRoute('library'); onKind(''); }}>
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
    </div>

    <div class="sidebar-section">
      <div class="sidebar-section-head">Saved searches</div>
      {#each savedSearches as saved}
        <button class="sidebar-item" type="button" onclick={() => onSavedSearch(saved.query, saved.name)}>
          <Icon name="bookmark" size={14} />
          <span class="truncate">{saved.name}</span>
        </button>
      {:else}
        <div class="sidebar-note">No saved searches yet</div>
      {/each}
    </div>

    <div class="sidebar-section bottom">
      <button class="sidebar-item is-disabled" type="button" disabled>
        <Icon name="settings" size={16} />
        <span>Settings API coming soon</span>
      </button>
      <button class:active={route === 'shortcuts'} class="sidebar-item" type="button" onclick={() => onRoute('shortcuts')}>
        <Icon name="keyboard" size={16} />
        <span>Shortcuts</span>
      </button>
    </div>
  </aside>

  {@render children()}
</div>
