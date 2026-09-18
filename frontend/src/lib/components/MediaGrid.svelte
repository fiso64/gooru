<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import MediaCard from './MediaCard.svelte';
  import PageNav from './PageNav.svelte';
  import { errorMessage } from '$lib/utils/format';
  import { isGridDirection, nextGridIndex } from '$lib/utils/gridNavigation';
  import { hasCommandModifier, isEditableShortcutTarget } from '$lib/utils/keyboard';
  import type { Snippet } from 'svelte';
  import { virtualGrid, virtualGridStartRow, virtualMediaGeometry, virtualMediaWindow } from '$lib/state/ui';
  import { effectiveGridSize, runtimeConfig } from '$lib/stores/runtimeConfig';
  import type { FileItem } from '$lib/api/types';

  const fitMediaInset = 20;

  let {
    sessionActive, isLoading, isError, error, files, retainedStartIndex, totalCount, displayTotalCount = totalCount, libraryCount,
    searchActive, selectedCount, bulkDownloadBusy = false, bulkDownloadError = '', isSelected, hasNextPage, isFetchingNextPage, hasPreviousPage,
    isFetchingPreviousPage, pagedMode = false, pageNumber = 1, pageCount = 1,
    loadMoreSentinel = $bindable<HTMLDivElement | undefined>(), onOpen,
    onToggleSelect, onExtendSelection, onSelectAll, onClearSelection, onBulkDownload, onBulkTag, onBulkUntag, onBulkUntrack,
    onBulkDelete, onLoadMore, onLoadPrevious, onPage, actions
  } = $props<{
    sessionActive: boolean; isLoading: boolean; isError: boolean; error: unknown; files: FileItem[];
    retainedStartIndex: number; totalCount: number; displayTotalCount?: number; libraryCount: number; searchActive: boolean;
    selectedCount: number; bulkDownloadBusy?: boolean; bulkDownloadError?: string;
    isSelected: (fileID: string) => boolean; hasNextPage: boolean;
    isFetchingNextPage: boolean; hasPreviousPage: boolean; isFetchingPreviousPage: boolean;
    pagedMode?: boolean; pageNumber?: number; pageCount?: number;
    loadMoreSentinel?: HTMLDivElement; onOpen: (file: FileItem, files: FileItem[]) => void;
    onToggleSelect: (file: FileItem, files: FileItem[], range: boolean) => void;
    onExtendSelection: (current: FileItem, target: FileItem, files: FileItem[]) => void; onSelectAll: () => void;
    onClearSelection: () => void; onBulkDownload: () => void | Promise<void>; onBulkTag: () => void; onBulkUntag: () => void;
    onBulkUntrack: () => void; onBulkDelete: () => void; onLoadMore: () => void | Promise<void>;
    onLoadPrevious: () => void | Promise<void>; onPage: (pageIndex: number) => void; actions?: Snippet;
  }>();

  let mainHost = $state<HTMLElement | undefined>();
  let gridHost = $state<HTMLDivElement | undefined>();
  let gridWidth = $state(960);
  let paneHeight = $state(900);
  let paneScrollY = $state(0);
  let gridTop = $state(0);
  let pixelRatio = $state(1);
  let tileAspectOverrides = $state<Record<string, number>>({});
  let initialViewportFocusDone = false;
  const tileMode = $derived($runtimeConfig.gridType === 'tile');
  const fitMode = $derived($runtimeConfig.gridType === 'fit');
  const layoutGridSize = $derived(effectiveGridSize($runtimeConfig.gridSize, $runtimeConfig.gridType));
  const virtualTotalCount = $derived(pagedMode ? files.length : totalCount || files.length);
  const virtualRetainedStartIndex = $derived(pagedMode ? 0 : retainedStartIndex);
  const retainedFileIDs = $derived(new Set(files.map((file: FileItem) => file.id)));
  const squareVirtual = $derived(virtualGrid(files, gridWidth, paneHeight, paneScrollY, gridTop, virtualTotalCount, virtualRetainedStartIndex, layoutGridSize));
  const tileGeometry = $derived(virtualMediaGeometry(files, gridWidth, virtualTotalCount, virtualRetainedStartIndex, layoutGridSize, tileAspectOverrides));
  const tileVirtual = $derived(virtualMediaWindow(tileGeometry, paneHeight, paneScrollY, gridTop, layoutGridSize));
  const tileVirtualHeight = $derived(pagedMode ? tileGeometry.localHeight : tileVirtual.totalHeight);

  onMount(() => { pixelRatio = Math.max(1, window.devicePixelRatio || 1); });

  function rememberThumbnailAspect(fileID: string, aspect: number) {
    if (!Number.isFinite(aspect) || aspect <= 0) return;
    const normalized = Math.min(8, Math.max(0.125, aspect));
    const retainedOverrides: Record<string, number> = {};
    let pruned = false;
    for (const [id, value] of Object.entries(tileAspectOverrides)) {
      if (retainedFileIDs.has(id)) retainedOverrides[id] = value;
      else pruned = true;
    }
    if (!pruned && Math.abs((retainedOverrides[fileID] ?? 0) - normalized) < 0.001) return;
    retainedOverrides[fileID] = normalized;
    tileAspectOverrides = retainedOverrides;
  }

  function squareScrollWindowKey(scrollY: number) {
    const rowHeight = squareVirtual.rowHeight;
    if (!Number.isFinite(rowHeight) || rowHeight <= 0) return '0:0';
    const viewportStart = Math.max(0, scrollY - gridTop);
    const startRow = virtualGridStartRow(scrollY, gridTop, rowHeight);
    const endRow = Math.ceil((viewportStart + paneHeight) / rowHeight);
    return `${startRow}:${endRow}`;
  }

  function handleScroll() {
    const nextScrollY = mainHost?.scrollTop ?? 0;
    if (tileMode) {
      const stride = Math.max(96, Math.floor(layoutGridSize * 0.75));
      if (Math.floor(Math.max(0, nextScrollY - gridTop) / stride) === Math.floor(Math.max(0, paneScrollY - gridTop) / stride)) return;
      paneScrollY = nextScrollY;
      return;
    }
    // The rendered end and transport look-ahead move before the overscanned start row does.
    // Keying only on the start row left short paged tails and infinite prefetch state stale
    // while the viewport advanced through those rows.
    if (squareScrollWindowKey(nextScrollY) === squareScrollWindowKey(paneScrollY)) return;
    paneScrollY = nextScrollY;
  }

  function focusFirstGridItem(event: KeyboardEvent) {
    if (event.defaultPrevented || event.key !== 'ArrowDown' || hasCommandModifier(event)) return;
    const target = event.target;
    const fromLibrarySearch = target instanceof HTMLElement && target.classList.contains('searchbar-input');
    if (isEditableShortcutTarget(target) && !fromLibrarySearch) return;
    const activeElement = document.activeElement;
    if (activeElement instanceof HTMLElement && activeElement.closest('[role="dialog"]')) return;
    if (activeElement instanceof HTMLElement && activeElement.classList.contains('thumb-open')) return;
    const first = gridHost?.querySelector<HTMLButtonElement>('.thumb-open');
    if (!first) return;
    event.preventDefault(); first.focus({ preventScroll: true }); first.scrollIntoView({ block: 'nearest', inline: 'nearest' });
  }

  function handleGridKeydown(event: KeyboardEvent) {
    if (!(event.target instanceof HTMLButtonElement) || !event.target.classList.contains('thumb-open')) return;
    if (event.key === 'Escape') {
      if (selectedCount > 0) return;
      event.preventDefault();
      event.stopPropagation();
      event.target.blur();
      return;
    }
    if (!isGridDirection(event.key)) return;
    const buttons = Array.from(gridHost?.querySelectorAll<HTMLButtonElement>('.thumb-open') ?? []);
    const currentIndex = buttons.indexOf(event.target);
    if (currentIndex < 0) return;
    const nextIndex = nextGridIndex(buttons.map((button) => button.getBoundingClientRect()), currentIndex, event.key, { wrapHorizontal: true });
    if (nextIndex === currentIndex) return;
    const nextButton = buttons[nextIndex];
    if (event.shiftKey) {
      const currentFile = files.find((file) => file.id === event.target.dataset.fileId);
      const targetFile = files.find((file) => file.id === nextButton?.dataset.fileId);
      if (currentFile && targetFile) onExtendSelection(currentFile, targetFile, files);
    }
    event.preventDefault(); event.stopPropagation(); nextButton?.focus({ preventScroll: true });
    nextButton?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
  }

  $effect(() => {
    const main = mainHost;
    if (initialViewportFocusDone || !main || !sessionActive || isLoading) return;
    const frame = requestAnimationFrame(() => {
      if (initialViewportFocusDone) return;
      const active = document.activeElement;
      if (active === document.body || active === document.documentElement || active === null) {
        main.focus({ preventScroll: true });
      }
      initialViewportFocusDone = true;
    });
    return () => cancelAnimationFrame(frame);
  });

  $effect(() => {
    const main = mainHost; const grid = gridHost; if (!main || !grid) return;
    let frame = 0;
    const measure = () => { frame = 0; gridWidth = grid.getBoundingClientRect().width; paneHeight = main.clientHeight; paneScrollY = main.scrollTop; gridTop = grid.offsetTop; };
    const schedule = () => { if (!frame) frame = requestAnimationFrame(measure); };
    const observer = new ResizeObserver(schedule); observer.observe(main); observer.observe(grid); measure();
    return () => { if (frame) cancelAnimationFrame(frame); observer.disconnect(); };
  });

  $effect(() => {
    if (pagedMode || tileMode) return;
    const node = loadMoreSentinel; const root = mainHost;
    if (!node || !root || !hasNextPage || isFetchingNextPage) return;
    const observer = new IntersectionObserver((entries) => { if (entries.some((entry) => entry.isIntersecting)) onLoadMore(); }, { root, rootMargin: '900px 0px' });
    observer.observe(node); return () => observer.disconnect();
  });

  $effect(() => {
    if (pagedMode) return;
    const needsPrevious = tileMode ? tileVirtual.needsPrevious : squareVirtual.needsPrevious;
    if (needsPrevious && hasPreviousPage && !isFetchingPreviousPage) onLoadPrevious();
  });
  $effect(() => {
    if (pagedMode) return;
    const needsNext = tileMode ? tileVirtual.needsNext : squareVirtual.needsNext;
    if (needsNext && hasNextPage && !isFetchingNextPage) onLoadMore();
  });

  $effect(() => {
    // Bookmarked/history pages can become invalid after the result set shrinks. An
    // out-of-range transport page is empty, which otherwise hides the pager and leaves
    // no in-UI route back to valid results. Valid deep pages always have files, so only
    // recover settled empty pages whose page number is beyond the known page count.
    if (!pagedMode || isLoading || isError || files.length || pageNumber <= pageCount) return;
    onPage(0);
  });

  function selectPagedPage(page: number) {
    if (page < 1 || page > pageCount || page === pageNumber || isFetchingNextPage || isFetchingPreviousPage) return;
    onPage(page - 1);
    mainHost?.scrollTo({ top: 0 });
    paneScrollY = 0;
  }
</script>

<svelte:window onkeydown={focusFirstGridItem} />
<main bind:this={mainHost} class="main" tabindex="-1" data-testid="library-viewport" onscroll={handleScroll}>
  {#if selectedCount > 0}
    <div class="selection-bar">
      <div class="selection-summary">
        <Icon name="check" size={14} active />
        <span><b>{selectedCount}</b> selected</span>
        {#if selectedCount < (totalCount || files.length)}
          <button class="g-btn g-btn-sm" type="button" onclick={onSelectAll}><span>Select</span><span style="margin-left: 0.3em"><u>a</u>ll {(totalCount || files.length).toLocaleString()}</span></button>
        {/if}
        {#if bulkDownloadError}<span class="selection-download-error" role="alert">{bulkDownloadError}</span>{/if}
      </div>
      <div class="sb-actions">
        <button class="g-btn g-btn-sm" type="button" disabled={bulkDownloadBusy} title="Download selected files" onclick={onBulkDownload}><Icon name="download" size={13} /> <u>D</u>ownload{bulkDownloadBusy ? '…' : ''}</button>
        <button class="g-btn g-btn-sm" type="button" onclick={onBulkTag}><Icon name="tag" size={13} /> <u>T</u>ag…</button>
        <button class="g-btn g-btn-sm" type="button" onclick={onBulkUntag}><Icon name="tag_remove" size={13} /> <u>U</u>ntag…</button>
        <button class="g-btn g-btn-sm" type="button" onclick={onBulkUntrack}><Icon name="untrack" size={13} /> Untrack</button>
        <button class="g-btn g-btn-sm" type="button" onclick={onBulkDelete}><Icon name="trash" size={13} /> Delete</button>
        <button class="g-btn g-btn-sm g-btn-icon" type="button" title="Clear" aria-label="Clear selection" onclick={onClearSelection}><Icon name="close" size={13} /></button>
      </div>
    </div>
  {/if}
  <div class="library-head"><div class="library-head-title"><h1>Library</h1><span class="library-head-meta" data-testid="library-header-count">{#if searchActive}{displayTotalCount.toLocaleString()} matching · {libraryCount.toLocaleString()} file{libraryCount === 1 ? '' : 's'}{:else}{(displayTotalCount || files.length).toLocaleString()} file{(displayTotalCount || files.length) === 1 ? '' : 's'}{/if}</span></div>{#if actions}{@render actions()}{/if}</div>

  {#if !sessionActive}<div class="empty-state"><p>Sign in to browse this library.</p></div>
  {:else if isLoading}<div class="grid skeleton-grid">{#each Array(18) as _}<div class="thumb skeleton"></div>{/each}</div>
  {:else if isError}<div class="empty-state error-state"><p>{errorMessage(error)}</p></div>
  {:else if !files.length}<div class="empty-state"><div class="empty-state-inner"><div class="empty-icon"><Icon name="search" size={24} /></div><h2>No results</h2><p>{#if searchActive}Nothing matches your filters. Try removing a pill, or check the tag spelling.{:else}Your library is empty. Drag files in, or run <code>gooru import</code> from a terminal.{/if}</p></div></div>
  {:else if !tileMode}
    <div bind:this={gridHost} class="virtual-grid" class:paged-virtual-grid={pagedMode} style={`height: ${squareVirtual.totalHeight}px;`}>
      <div class={`grid${fitMode ? ' fit-media-grid' : ''}`} role="group" aria-label="Media grid" data-testid="virtual-media-grid" data-grid-type={$runtimeConfig.gridType} style={`transform: translateY(${squareVirtual.offsetTop}px);`}>
        {#if isFetchingPreviousPage}<div class="thumb skeleton"></div>{/if}
        {#each squareVirtual.files as file (file.id)}<MediaCard {file} cardWidth={squareVirtual.cardWidth} {pixelRatio} viewportRoot={mainHost} fitMedia={fitMode} mediaInset={fitMode ? fitMediaInset : 0} selected={isSelected(file.id)} selectionActive={selectedCount > 0} onOpen={(opened) => onOpen(opened, files)} onToggleSelect={(target, range) => onToggleSelect(target, files, range)} onGridKeydown={handleGridKeydown} />{/each}
      </div>
    </div>
  {:else}
    <div bind:this={gridHost} class="virtual-grid" class:paged-virtual-grid={pagedMode} style={`height: ${tileVirtualHeight}px;`}>
      <div class="grid variable-media-grid" role="group" aria-label="Media grid" data-testid="virtual-media-grid" data-grid-type="tile">
        {#each tileVirtual.items as item (item.file.id)}<div class="virtual-media-item" style={`left:${item.x}px;top:${item.y}px;width:${item.width}px;height:${item.height}px`}><MediaCard file={item.file} cardWidth={item.width} cardHeight={item.height} {pixelRatio} viewportRoot={mainHost} fitMedia selected={isSelected(item.file.id)} selectionActive={selectedCount > 0} onOpen={(opened) => onOpen(opened, files)} onToggleSelect={(target, range) => onToggleSelect(target, files, range)} onThumbnailAspect={rememberThumbnailAspect} onGridKeydown={handleGridKeydown} /></div>{/each}
      </div>
    </div>
  {/if}
  {#if pagedMode && files.length}
    <PageNav
      page={pageNumber}
      {pageCount}
      ariaLabel="Library pages"
      testId="library-pager"
      disabled={isFetchingPreviousPage || isFetchingNextPage}
      onPage={selectPagedPage}
    />
  {:else if !pagedMode && (hasNextPage || isFetchingNextPage)}
    <div bind:this={loadMoreSentinel} class="infinite-sentinel" data-testid="infinite-scroll-sentinel"><span class="infinite-sentinel-content"><span class="infinite-sentinel-spinner" aria-hidden="true"></span>Loading more</span></div>
  {/if}
</main>