<script lang="ts">
  import Icon from './Icon.svelte';
  import MediaCard from './MediaCard.svelte';
  import { errorMessage } from '$lib/utils/format';
  import type { Snippet } from 'svelte';
  import { virtualGrid } from '$lib/state/ui';
  import type { FileItem } from '$lib/api/types';

  let {
    sessionActive,
    isLoading,
    isError,
    error,
    files,
    retainedStartIndex,
    totalCount,
    libraryCount,
    searchActive,
    selectedIDs,
    hasNextPage,
    isFetchingNextPage,
    hasPreviousPage,
    isFetchingPreviousPage,
    loadMoreSentinel = $bindable<HTMLDivElement | undefined>(),
    onOpen,
    onToggleSelect,
    onSelectAll,
    onClearSelection,
    onBulkTag,
    onBulkUntag,
    onLoadMore,
    onLoadPrevious,
    actions
  } = $props<{
    sessionActive: boolean;
    isLoading: boolean;
    isError: boolean;
    error: unknown;
    files: FileItem[];
    retainedStartIndex: number;
    totalCount: number;
    libraryCount: number;
    searchActive: boolean;
    selectedIDs: Set<string>;
    hasNextPage: boolean;
    isFetchingNextPage: boolean;
    hasPreviousPage: boolean;
    isFetchingPreviousPage: boolean;
    loadMoreSentinel?: HTMLDivElement;
    onOpen: (file: FileItem) => void;
    onToggleSelect: (file: FileItem) => void;
    onSelectAll: () => void;
    onClearSelection: () => void;
    onBulkTag: () => void;
    onBulkUntag: () => void;
    onLoadMore: () => void;
    onLoadPrevious: () => void;
    actions?: Snippet;
  }>();

  let mainHost = $state<HTMLElement | undefined>();
  let gridHost = $state<HTMLDivElement | undefined>();
  let gridWidth = $state(960);
  let paneHeight = $state(900);
  let paneScrollY = $state(0);
  let gridTop = $state(0);
  const virtual = $derived(virtualGrid(files, gridWidth, paneHeight, paneScrollY, gridTop, totalCount || files.length, retainedStartIndex));

  function handleScroll() {
    paneScrollY = mainHost?.scrollTop ?? 0;
  }

  $effect(() => {
    const main = mainHost;
    const grid = gridHost;
    if (!main || !grid) return;

    let frame = 0;
    const measure = () => {
      frame = 0;
      gridWidth = grid.getBoundingClientRect().width;
      paneHeight = main.clientHeight;
      paneScrollY = main.scrollTop;
      gridTop = grid.offsetTop;
    };
    const schedule = () => {
      if (frame) return;
      frame = requestAnimationFrame(measure);
    };
    const observer = new ResizeObserver(schedule);
    observer.observe(main);
    observer.observe(grid);
    measure();
    return () => {
      if (frame) cancelAnimationFrame(frame);
      observer.disconnect();
    };
  });

  $effect(() => {
    const node = loadMoreSentinel;
    const root = mainHost;
    if (!node || !root || !hasNextPage || isFetchingNextPage) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) onLoadMore();
      },
      { root, rootMargin: '900px 0px' }
    );
    observer.observe(node);
    return () => observer.disconnect();
  });

  $effect(() => {
    if (virtual.needsPrevious && hasPreviousPage && !isFetchingPreviousPage) onLoadPrevious();
  });

  $effect(() => {
    if (virtual.needsNext && hasNextPage && !isFetchingNextPage) onLoadMore();
  });
</script>

<main bind:this={mainHost} class="main" onscroll={handleScroll}>
  {#if selectedIDs.size > 0}
    <div class="selection-bar">
      <div class="selection-summary">
        <Icon name="check" size={14} active />
        <span><b>{selectedIDs.size}</b> of <span>{totalCount || files.length}</span> selected</span>
        {#if selectedIDs.size < files.length}
          <button class="g-btn g-btn-sm" type="button" onclick={onSelectAll}>
            {files.length === (totalCount || files.length) ? `Select all ${files.length}` : `Select all loaded ${files.length}`}
          </button>
        {/if}
      </div>
      <div class="sb-actions">
        <button class="g-btn g-btn-sm" type="button" onclick={onBulkTag}><Icon name="tag" size={13} /> Tag…</button>
        <button class="g-btn g-btn-sm" type="button" disabled title="Export bundles are not supported yet"><Icon name="download" size={13} /> Export</button>
        <button class="g-btn g-btn-sm" type="button" onclick={onBulkUntag}><Icon name="trash" size={13} /> Untag…</button>
        <button class="g-btn g-btn-sm g-btn-icon" type="button" title="Clear" aria-label="Clear selection" onclick={onClearSelection}><Icon name="close" size={13} /></button>
      </div>
    </div>
  {/if}

  <div class="library-head">
    <div class="library-head-title">
      <h1>Library</h1>
      <span class="library-head-meta">
        {(totalCount || files.length).toLocaleString()} file{(totalCount || files.length) === 1 ? '' : 's'}
        {#if searchActive && libraryCount} · filtered from {libraryCount.toLocaleString()}{/if}
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
      <div class="empty-state-inner">
        <div class="empty-icon"><Icon name="search" size={24} /></div>
        <h2>No results</h2>
        <p>
          {#if searchActive}
            Nothing matches your filters. Try removing a pill, or check the tag spelling.
          {:else}
            Your library is empty. Drag files in, or run <code>gooru import</code> from a terminal.
          {/if}
        </p>
      </div>
    </div>
  {:else}
    <div bind:this={gridHost} class="virtual-grid" style={`height: ${virtual.totalHeight}px;`}>
      <div class="grid" data-testid="virtual-media-grid" style={`transform: translateY(${virtual.offsetTop}px);`}>
        {#if isFetchingPreviousPage}
          <div class="thumb skeleton"></div>
        {/if}
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
        <span class="infinite-sentinel-content">
          <span class="infinite-sentinel-spinner" aria-hidden="true"></span>
          Loading more
        </span>
      </div>
    {/if}
  {/if}
</main>
