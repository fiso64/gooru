<script lang="ts">
  import Icon from './Icon.svelte';
  import MediaCard from './MediaCard.svelte';
  import { errorMessage } from '$lib/utils/format';
  import type { Snippet } from 'svelte';
  import type { VirtualGrid } from '$lib/state/ui';
  import type { FileItem } from '$lib/api/types';

  let {
    sessionActive,
    isLoading,
    isError,
    error,
    files,
    virtual,
    totalCount,
    libraryCount,
    searchActive,
    selectedIDs,
    hasNextPage,
    isFetchingNextPage,
    loadMoreSentinel = $bindable<HTMLDivElement | undefined>(),
    onOpen,
    onToggleSelect,
    onSelectAll,
    onClearSelection,
    onBulkTag,
    actions
  } = $props<{
    sessionActive: boolean;
    isLoading: boolean;
    isError: boolean;
    error: unknown;
    files: FileItem[];
    virtual: VirtualGrid;
    totalCount: number;
    libraryCount: number;
    searchActive: boolean;
    selectedIDs: Set<string>;
    hasNextPage: boolean;
    isFetchingNextPage: boolean;
    loadMoreSentinel?: HTMLDivElement;
    onOpen: (file: FileItem) => void;
    onToggleSelect: (file: FileItem) => void;
    onSelectAll: () => void;
    onClearSelection: () => void;
    onBulkTag: () => void;
    actions?: Snippet;
  }>();
</script>

<main class="main">
  {#if selectedIDs.size > 0}
    <div class="selection-bar">
      <div>
        <Icon name="check" size={14} active />
        <span><b>{selectedIDs.size}</b> of <span>{totalCount || files.length}</span> selected</span>
        {#if selectedIDs.size < files.length}
          <button class="g-btn g-btn-sm" type="button" onclick={onSelectAll}>Select loaded</button>
        {/if}
      </div>
      <div class="sb-actions">
        <button class="g-btn g-btn-sm" type="button" onclick={onBulkTag}><Icon name="tag" size={13} /> Tag</button>
        <button class="g-btn g-btn-sm" type="button" disabled title="Export bundles are not supported yet"><Icon name="download" size={13} /> Export</button>
        <button class="g-btn g-btn-sm g-btn-icon" type="button" title="Clear" aria-label="Clear selection" onclick={onClearSelection}><Icon name="close" size={13} /></button>
      </div>
    </div>
  {/if}

  <div class="library-head">
    <div class="library-head-title">
      <h1>Library</h1>
      <span class="library-head-meta">
        {(totalCount || files.length).toLocaleString()} file{(totalCount || files.length) === 1 ? '' : 's'}
        {#if searchActive && libraryCount} filtered from {libraryCount.toLocaleString()}{/if}
      </span>
    </div>
    {#if actions}{@render actions()}{/if}
  </div>

  {#if !sessionActive}
    <div class="empty-state"><p>Sign in to browse this library.</p></div>
  {:else if isLoading}
    <div class="grid skeleton-grid">
      {#each Array(18) as _}<div class="thumb skeleton"></div>{/each}
    </div>
  {:else if isError}
    <div class="empty-state error-state"><p>{errorMessage(error)}</p></div>
  {:else if !files.length}
    <div class="empty-state">
      <div class="empty-icon"><Icon name="search" size={24} /></div>
      <h2>No results</h2>
      <p>{searchActive ? 'Nothing matches the active filters.' : 'Your library is empty. Import files to start browsing.'}</p>
    </div>
  {:else}
    <div class="virtual-grid" style={`height: ${virtual.totalHeight}px;`}>
      <div class="grid" data-testid="virtual-media-grid" style={`transform: translateY(${virtual.offsetTop}px);`}>
        {#each virtual.files as file (file.id)}
          <MediaCard
            {file}
            selected={selectedIDs.has(file.id)}
            onOpen={onOpen}
            onToggleSelect={onToggleSelect}
          />
        {/each}
      </div>
    </div>
    {#if hasNextPage || isFetchingNextPage}
      <div bind:this={loadMoreSentinel} class="infinite-sentinel" data-testid="infinite-scroll-sentinel">
        {isFetchingNextPage ? 'Loading more results' : 'More results available'}
      </div>
    {/if}
  {/if}
</main>
