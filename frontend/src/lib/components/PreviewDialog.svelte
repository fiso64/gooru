<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import Icon from './Icon.svelte';
  import ViewerSidebar from './ViewerSidebar.svelte';
  import ViewerStage from './ViewerStage.svelte';
  import { ApiClient } from '$lib/api/client';
  import { hasRuntimeCapability, runtimeCapability, runtimeConfig } from '$lib/stores/runtimeConfig';
  import { readViewerSessionPreferences, updateViewerSessionPreferences } from '$lib/state/viewerSessionPreferences';
  import { comicPageAt, isComicFile, moveComicPage } from '$lib/utils/comic';
  import { errorMessage, formatBytes, mediaDimensions, mediaDuration } from '$lib/utils/format';
  import { claimFocus } from '$lib/utils/focus';
  import { hasCommandModifier, isEditableShortcutTarget } from '$lib/utils/keyboard';
  import { hasBlockingModal } from '$lib/utils/modal';
  import { isEmptyViewerTagShortcut } from '$lib/utils/viewerTagKeyRouting';
  import { canUseOriginalInViewer, viewerImageSource } from '$lib/utils/media';
  import { clearViewerPreloadCache, preloadViewerMediaSource } from '$lib/utils/viewerPreload';
  import type { ComicManifest, FileItem } from '$lib/api/types';

  let {
    file,
    preloadPrev,
    preloadNext,
    navigationError = '',
    tagDraft,
    tagBusy,
    tagError,
    tags,
    onClose,
    onPrev,
    onNext,
    onTagInput,
    onMutateTags,
    onRemoveTag,
    onTagSearch,
    onUntrack,
    onDelete,
    onNestedNavigationChange = () => undefined
  } = $props<{
    file: FileItem;
    preloadPrev?: FileItem;
    preloadNext?: FileItem;
    navigationError?: string;
    tagDraft: string;
    tagBusy: boolean;
    tagError: string;
    tags: Array<{ name?: string; tag?: string; namespace?: string; value?: string; count?: number }>;
    onClose: () => void;
    onPrev: () => void | Promise<void>;
    onNext: () => void | Promise<void>;
    onTagInput: (fileID: string, value: string) => void;
    onMutateTags: (file: FileItem, operation: 'add' | 'set' | 'remove', value?: string) => void;
    onRemoveTag: (file: FileItem, tag: string) => void;
    onTagSearch: (tag: string) => void;
    onUntrack: (file: FileItem) => void;
    onDelete: (file: FileItem) => void;
    onNestedNavigationChange?: (active: boolean) => void;
  }>();

  const initialViewerPreferences = readViewerSessionPreferences({
    preferOriginal: $runtimeConfig.loadFullMediaByDefault,
    rotation: 0,
    fitMode: 'fit_window',
    scaling: $runtimeConfig.viewerScaling
  });

  let dialogElement = $state<HTMLDivElement | undefined>();
  let downloadLink = $state<HTMLAnchorElement | undefined>();
  let openOriginalLink = $state<HTMLAnchorElement | undefined>();
  let preferOriginal = $state(initialViewerPreferences.preferOriginal);
  let tagMode = $state<'add' | 'remove'>('add');
  let comicManifest = $state<ComicManifest | null>(null);
  let comicPageIndex = $state(0);
  let comicEntered = $state(false);
  let comicLoading = $state(false);
  let comicError = $state('');
  let comicController: AbortController | undefined;
  let navigationDirection: -1 | 1 = 1;
  let presentedSource = $state('');
  let primedNeighborID = '';


  const originalAvailable = $derived(canUseOriginalInViewer(file));
  const previewAvailable = $derived(hasRuntimeCapability($runtimeConfig, runtimeCapability.previewImages));
  const effectivePreferOriginal = $derived(!previewAvailable || preferOriginal);
  const comicAvailable = $derived(isComicFile(file));
  const currentComicPage = $derived(comicPageAt(comicManifest, comicPageIndex));
  const imageSource = $derived(comicEntered && currentComicPage ? currentComicPage.url : viewerImageSource(file, effectivePreferOriginal));
  const sidebarMetadata = $derived.by(() => {
    const dimensionLabel = mediaDimensions(file);
    const pageLabel = file.metadata.page_count
      ? `${file.metadata.page_count} ${file.metadata.page_count === 1 ? 'page' : 'pages'} · `
      : dimensionLabel
        ? `${dimensionLabel} · `
        : '';
    const rows: Array<{ label: string; value: string; className?: string }> = [
      { label: 'Path', value: file.safe_display_path, className: 'path' },
      { label: 'Size', value: `${pageLabel}${formatBytes(file.size)}` }
    ];
    const duration = mediaDuration(file);
    if (duration) rows.push({ label: 'Length', value: duration });
    rows.push(
      { label: 'Added', value: modifiedLabel(file.added_at) },
      { label: 'Modified', value: modifiedLabel(file.modified_time) },
      { label: 'Mime', value: file.media_type },
      { label: 'Id', value: file.content_id, className: 'hash' }
    );
    return rows;
  });

  onMount(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : undefined;
    if ($runtimeConfig.fullscreenMediaByDefault) {
      const stage = dialogElement?.querySelector<HTMLElement>('.viewer-stage');
      if (stage) void stage.requestFullscreen().catch(() => undefined);
    }
    return claimFocus(dialogElement, previous);
  });

  $effect(() => {
    file.id;
    untrack(() => {
      comicController?.abort();
      comicController = undefined;
      comicManifest = null;
      comicPageIndex = 0;
      comicEntered = false;
      comicLoading = false;
      comicError = '';
      tagMode = 'add';
      onNestedNavigationChange(false);
    });
  });

  $effect(() => {
    // Any requested source change invalidates speculative work immediately. The next neighbor
    // is not primed until ViewerStage confirms that this exact target has been presented.
    file.id;
    imageSource;
    comicEntered;
    comicPageIndex;
    presentedSource = '';
    primedNeighborID = '';
    clearViewerPreloadCache();
  });

  // On a cold viewer open the server can resolve the neighbor after the media
  // is already visible. Prime it then as well, without waiting for a second
  // presentation event. Do not repeatedly cancel an unchanged completed preload.
  $effect(() => {
    const source = presentedSource;
    const neighbor = navigationDirection < 0 ? preloadPrev : preloadNext;
    const id = neighbor?.id ?? '';
    if (!source || source !== imageSource || !neighbor || comicEntered || id === primedNeighborID) return;
    primedNeighborID = id;
    void preloadViewerMediaSource(neighbor, viewerImageSource(neighbor, effectivePreferOriginal)).catch(() => undefined);
  });

  function primeAfterPresentation(source: string) {
    if (source !== imageSource) return;
    presentedSource = source;
    primedNeighborID = '';
    clearViewerPreloadCache();

    if (comicEntered && comicManifest) {
      const targetIndex = comicPageIndex + navigationDirection;
      const page = comicPageAt(comicManifest, targetIndex);
      if (page) void preloadViewerMediaSource(file, page.url).catch(() => undefined);
      return;
    }

    // The reactive scheduler also handles neighbors arriving after presentation.
    const neighbor = navigationDirection < 0 ? preloadPrev : preloadNext;
    if (!neighbor) return;
    primedNeighborID = neighbor.id;
    void preloadViewerMediaSource(neighbor, viewerImageSource(neighbor, effectivePreferOriginal)).catch(() => undefined);
  }

  function focusTagInput(mode: 'add' | 'remove' = 'add') {
    tagMode = mode;
    document.getElementById(`tags-${file.id}`)?.focus();
  }

  function toggleViewerFullscreen() {
    // Reuse ViewerStage's existing fullscreen button so the rail action follows the same
    // enter/exit behavior and fullscreen-by-default close guard as the F shortcut/control.
    dialogElement
      ?.querySelector<HTMLButtonElement>('.viewer-stage .viewer-mode-button[aria-label="Toggle fullscreen"]')
      ?.click();
  }

  function toggleOriginalMedia() {
    if (!previewAvailable || !originalAvailable) return;
    preferOriginal = !preferOriginal;
    updateViewerSessionPreferences({ preferOriginal });
  }

  function handleViewerKeydown(event: KeyboardEvent) {
    if (event.defaultPrevented || hasBlockingModal() || hasCommandModifier(event)) return;
    const viewerTagShortcut = isEmptyViewerTagShortcut(event, file.id);
    if (
      (event.key === 'ArrowLeft' || event.key === 'ArrowRight')
      && viewerTagShortcut
      && event.target instanceof HTMLInputElement
      && !event.shiftKey
    ) {
      event.preventDefault();
      event.stopPropagation();
      event.target.blur();
      // Navigation can await a server-side window lookup. Focus the new file's
      // tag input only after the selected file has actually changed.
      const navigation = event.key === 'ArrowLeft' ? stagePrev() : stageNext();
      void Promise.resolve(navigation).then(() => tick()).then(() => focusTagInput()).catch(() => undefined);
      return;
    }

    if (isEditableShortcutTarget(event.target) && !viewerTagShortcut) return;
    const key = event.key.toLowerCase();
    if (key === 't' || key === 'u') {
      event.preventDefault();
      event.stopPropagation();
      focusTagInput(key === 'u' ? 'remove' : 'add');
      return;
    }
    if (key === 'q' && previewAvailable && originalAvailable) {
      event.preventDefault();
      event.stopPropagation();
      toggleOriginalMedia();
      return;
    }
    if (key === 'd') {
      event.preventDefault();
      event.stopPropagation();
      downloadLink?.click();
      return;
    }
    if (key === 'o') {
      event.preventDefault();
      event.stopPropagation();
      openOriginalLink?.click();
      return;
    }
    if (event.key === 'Delete') {
      event.preventDefault();
      event.stopPropagation();
      if (event.shiftKey) {
        if (file.can_delete) onDelete(file);
      } else {
        onUntrack(file);
      }
    }
  }

  async function toggleComic() {
    if (!comicAvailable || comicLoading) return;
    if (comicEntered) {
      comicEntered = false;
      onNestedNavigationChange(false);
      return;
    }

    if (!comicManifest) {
      comicController?.abort();
      const controller = new AbortController();
      comicController = controller;
      comicLoading = true;
      comicError = '';
      const fileID = file.id;
      try {
        const manifest = await new ApiClient().getComicManifest(fileID, controller.signal);
        if (controller.signal.aborted || file.id !== fileID) return;
        comicManifest = manifest;
      } catch (error) {
        if (!controller.signal.aborted && file.id === fileID) comicError = errorMessage(error);
        return;
      } finally {
        if (comicController === controller) {
          comicController = undefined;
          comicLoading = false;
        }
      }
    }

    if (!comicManifest?.pages.length) {
      comicError = 'This comic has no readable image pages.';
      return;
    }
    comicPageIndex = 0;
    comicEntered = true;
    onNestedNavigationChange(true);
  }

  function movePage(delta: number) {
    const next = moveComicPage(comicPageIndex, delta, comicManifest?.pages.length ?? 0);
    if (next !== comicPageIndex) {
      navigationDirection = delta < 0 ? -1 : 1;
      comicPageIndex = next;
    }
  }

  function stagePrev() {
    navigationDirection = -1;
    if (comicEntered) movePage(-1);
    else return onPrev();
  }

  function stageNext() {
    navigationDirection = 1;
    if (comicEntered) movePage(1);
    else return onNext();
  }

  function handleBackdropKeydown(event: KeyboardEvent) {
    if (event.target !== event.currentTarget || event.key !== 'Escape') return;
    event.preventDefault();
    event.stopPropagation();
    onClose();
  }

  function modifiedLabel(value: string) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;
    return date.toLocaleString('en-GB', { year: 'numeric', month: 'short', day: '2-digit', hour: '2-digit', minute: '2-digit' });
  }
</script>

<svelte:window onkeydown={handleViewerKeydown} />

<div
  bind:this={dialogElement}
  class="lightbox"
  role="dialog"
  aria-modal="true"
  aria-labelledby="preview-title"
  tabindex="-1"
  onclick={(event) => { if (event.target === event.currentTarget) onClose(); }}
  onkeydown={handleBackdropKeydown}
>
  <ViewerSidebar
    titleID="preview-title"
    eyebrow={file.media_kind}
    fileID={file.id}
    fileName={file.name}
    metadata={sidebarMetadata}
    tagValues={file.tags}
    tagCandidates={tags}
    {tagDraft}
    {tagBusy}
    {tagError}
    {tagMode}
    {onClose}
    onTagInput={(value) => onTagInput(file.id, value)}
    onCommitTag={(value) => onMutateTags(file, tagMode, value)}
    onRemoveTag={(tag) => onRemoveTag(file, tag)}
    onModeToggle={() => focusTagInput(tagMode === 'add' ? 'remove' : 'add')}
    {onTagSearch}
  />

  <ViewerStage
    {file}
    {imageSource}
    initialFitMode={$runtimeConfig.viewerFitMode}
    boundActualSizeToFit={$runtimeConfig.viewerActualSizeFitCap}
    initialScaling={$runtimeConfig.viewerScaling}
    onPrev={stagePrev}
    onNext={stageNext}
    onPrimaryAction={comicAvailable ? () => void toggleComic() : undefined}
    keyboardNavigation={comicEntered}
    navigationUnit={comicEntered ? 'page' : 'file'}
    closeOnFullscreenExit={$runtimeConfig.fullscreenMediaByDefault}
    onFullscreenExit={onClose}
    {comicAvailable}
    {comicEntered}
    {comicLoading}
    comicPage={comicPageIndex}
    comicPages={comicManifest?.pages.length ?? 0}
    {comicError}
    {navigationError}
    onToggleComic={() => void toggleComic()}
    onComicPageSelect={(index) => {
      navigationDirection = index < comicPageIndex ? -1 : 1;
      comicPageIndex = index;
    }}
    onPresented={primeAfterPresentation}
  />

  <aside class="lightbox-rail">
    <button class="g-btn g-btn-ghost" type="button" title="Fullscreen (F)" aria-label="Toggle fullscreen" onclick={toggleViewerFullscreen}>
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <path d="M8 3H3v5M16 3h5v5M8 21H3v-5M16 21h5v-5" />
      </svg>
    </button>
    {#if previewAvailable && originalAvailable}
      <button
        class="g-btn g-btn-ghost"
        type="button"
        aria-label={preferOriginal ? 'Use derived preview' : 'Use original media'}
        aria-pressed={preferOriginal}
        title={preferOriginal ? 'Use derived preview (Q)' : 'Use original media (Q)'}
        onclick={toggleOriginalMedia}
      >
        <Icon name="photo" size={16} active={preferOriginal} />
      </button>
    {/if}
    <a bind:this={downloadLink} class="g-btn g-btn-ghost" href={file.media_urls.download || file.media_urls.content} title="Download original (D)" aria-label={`Download ${file.name}`}>
      <Icon name="download" size={16} />
    </a>
    <a bind:this={openOriginalLink} class="g-btn g-btn-ghost" href={file.media_urls.content} target="_blank" rel="noreferrer" title="Open original in new tab (O)" aria-label={`Open original ${file.name}`}>
      <Icon name="external" size={16} />
    </a>
    <div class="rail-spacer"></div>
    <button class="g-btn g-btn-ghost" type="button" disabled title="Additional info coming soon" aria-label="Info"><Icon name="info" size={16} /></button>
    {#if file.can_delete}
      <button class="g-btn g-btn-ghost" type="button" title="Delete file from disk (Shift+Delete)" aria-label={`Delete ${file.name} from disk`} onclick={() => onDelete(file)}><Icon name="trash" size={16} /></button>
    {/if}
    <button class="g-btn g-btn-ghost" type="button" title="Remove from library without deleting the file (Delete)" aria-label={`Untrack ${file.name} from library`} onclick={() => onUntrack(file)}><Icon name="close" size={16} /></button>
  </aside>
</div>

<style>
  :global(.lightbox-rail .g-btn[aria-pressed='true']) {
    color: var(--accent);
    background: var(--accent-soft);
    border-color: var(--accent-line);
  }
</style>
