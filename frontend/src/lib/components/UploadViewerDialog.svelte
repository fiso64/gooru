<script lang="ts">
  import { untrack } from 'svelte';
  import Icon from './Icon.svelte';
  import PreviewDialog from './PreviewDialog.svelte';
  import ViewerSidebar from './ViewerSidebar.svelte';
  import ViewerStage from './ViewerStage.svelte';
  import { ApiClient } from '$lib/api/client';
  import type { FileItem } from '$lib/api/types';
  import { authState } from '$lib/stores/auth';
  import { runtimeConfig } from '$lib/stores/runtimeConfig';
  import type { UploadItem } from '$lib/state/uploadItems';
  import { uploadItemHasLocalViewer, uploadViewerNeighborIndex, type UploadViewerScope } from '$lib/state/uploadViewer';
  import { errorMessage, formatBytes, parseTags } from '$lib/utils/format';
  import { matchesShortcutModifiers, isEditableShortcutTarget } from '$lib/utils/keyboard';
  import { viewerImageSource, type ViewerStageMedia } from '$lib/utils/media';
  import type { TagCandidate } from '$lib/utils/tagSuggestions';

  let {
    uploadItems,
    activeIndex,
    scope,
    tags,
    onIndex,
    onClose,
    onItemTagsInput,
    onItemRemoteTagsLoaded,
    onRemove,
    onRemoteUntrack,
    onRemoteDelete,
    onTagSearch
  } = $props<{
    uploadItems: UploadItem[];
    activeIndex: number;
    scope: UploadViewerScope;
    tags: TagCandidate[];
    onIndex: (index: number) => void;
    onClose: () => void;
    onItemTagsInput: (index: number, tags: string[]) => void;
    onItemRemoteTagsLoaded: (index: number, tags: string[], expectedBaseTags: string[] | undefined) => void;
    onRemove: (index: number) => void;
    onRemoteUntrack: (file: FileItem) => void;
    onRemoteDelete: (file: FileItem) => void;
    onTagSearch: (tag: string) => void;
  }>();

  let dialogElement = $state<HTMLDivElement | undefined>();
  let localURL = $state('');
  let remoteFile = $state<FileItem | undefined>();
  let remoteLoading = $state(false);
  let remoteError = $state('');
  let remoteNestedNavigation = $state(false);
  let observedItem = $state<UploadItem | undefined>();
  let tagDraft = $state('');
  let tagMode = $state<'add' | 'remove'>('add');
  let tagOverride = $state<string[] | undefined>();
  let observedIndex = $state(-1);

  const activeItem = $derived(uploadItems[activeIndex]);
  const localMedia = $derived.by<ViewerStageMedia | undefined>(() => {
    const item = activeItem;
    if (!item || !localURL || !uploadItemHasLocalViewer(item)) return undefined;
    const mediaType = (item.previewFile?.type || item.type || 'application/octet-stream').trim().toLowerCase();
    const mediaKind = mediaType.startsWith('video/')
      ? 'video'
      : mediaType.startsWith('audio/')
        ? 'audio'
        : mediaType === 'image/gif'
          ? 'gif'
          : 'photo';
    return {
      id: `upload-local-${activeIndex}`,
      name: item.name,
      viewer_support: 'supported',
      media_kind: mediaKind,
      media_type: mediaType,
      media_urls: { content: localURL, preview: localURL },
      metadata: {}
    };
  });
  const currentTags = $derived(tagOverride ?? activeItem?.tags ?? remoteFile?.tags ?? []);
  const remoteViewerFile = $derived(remoteFile ? { ...remoteFile, tags: currentTags } : undefined);
  const imageSource = $derived(localMedia ? viewerImageSource(localMedia, false) : '');
  const kindLabel = $derived(localMedia?.media_kind ?? 'media');
  const tagEditorID = $derived(`upload-${activeIndex}`);
  const sidebarMetadata = $derived.by(() => {
    const item = activeItem;
    if (!item) return [];
    const rows: Array<{ label: string; value: string; className?: string }> = [
      { label: 'Size', value: formatBytes(item.size) },
      { label: 'Status', value: item.status.replace(/_/g, ' ') },
      { label: 'Mime', value: item.type || 'unknown' }
    ];
    if (item.batchID != null) rows.push({ label: 'Batch', value: String(item.batchID) });
    return rows;
  });

  $effect(() => {
    const item = activeItem;
    if (!item) {
      onClose();
      return;
    }
    if (activeIndex === observedIndex) {
      if (observedItem && item !== observedItem) onClose();
      return;
    }
    observedIndex = activeIndex;
    observedItem = item;
    tagDraft = '';
    tagMode = 'add';
    tagOverride = undefined;
    remoteNestedNavigation = false;
  });

  $effect(() => {
    const item = activeItem;
    const file = item?.previewFile;
    if (!item || !file || !uploadItemHasLocalViewer(item)) {
      localURL = '';
      return;
    }
    const url = URL.createObjectURL(file);
    localURL = url;
    return () => URL.revokeObjectURL(url);
  });

  $effect(() => {
    const remoteFileID = activeItem?.remoteFileID;
    remoteFile = undefined;
    remoteError = '';
    remoteLoading = Boolean(remoteFileID);
    if (!remoteFileID) return;

    const requestIndex = activeIndex;
    const expectedBaseTags = untrack(() => activeItem?.tagSyncBaseTags ? [...activeItem.tagSyncBaseTags] : undefined);
    const controller = new AbortController();
    const client = new ApiClient($authState.csrfToken);
    void client.getFile(remoteFileID, controller.signal)
      .then((file) => {
        if (controller.signal.aborted) return;
        remoteFile = file;
        onItemRemoteTagsLoaded(requestIndex, file.tags ?? [], expectedBaseTags);
        tagOverride = undefined;
      })
      .catch((error) => {
        if (!controller.signal.aborted) remoteError = errorMessage(error);
      })
      .finally(() => {
        if (!controller.signal.aborted) remoteLoading = false;
      });
    return () => controller.abort();
  });

  function move(delta: -1 | 1) {
    const nextIndex = uploadViewerNeighborIndex(uploadItems, activeIndex, scope, delta);
    if (nextIndex != null && nextIndex !== activeIndex) onIndex(nextIndex);
  }

  function setDesiredTags(nextTags: string[]) {
    tagOverride = nextTags;
    if (remoteFile) remoteFile = { ...remoteFile, tags: nextTags };
    onItemTagsInput(activeIndex, nextTags);
  }

  function commitTag(value: string, mode = tagMode) {
    const changed = parseTags(value);
    if (!changed.length) return;
    if (mode === 'remove') {
      const removed = new Set(changed);
      setDesiredTags(currentTags.filter((tag: string) => !removed.has(tag)));
    } else {
      setDesiredTags(Array.from(new Set([...currentTags, ...changed])));
    }
    tagDraft = '';
  }

  function removeTag(tag: string) {
    setDesiredTags(currentTags.filter((candidate: string) => candidate !== tag));
  }

  function focusTagInput(mode: 'add' | 'remove' = 'add') {
    tagMode = mode;
    document.getElementById(`tags-${tagEditorID}`)?.focus();
  }

  function toggleViewerFullscreen() {
    dialogElement
      ?.querySelector<HTMLButtonElement>('.viewer-stage .viewer-mode-button[aria-label="Toggle fullscreen"]')
      ?.click();
  }

  function removeStagedItem() {
    if (activeItem?.status !== 'staged') return;
    const index = activeIndex;
    onClose();
    onRemove(index);
  }

  function handleBackdropKeydown(event: KeyboardEvent) {
    if (event.target !== event.currentTarget || event.key !== 'Escape' || !matchesShortcutModifiers(event)) return;
    event.preventDefault();
    event.stopPropagation();
    onClose();
  }

  function handleWindowKeydown(event: KeyboardEvent) {
    if (event.defaultPrevented || !matchesShortcutModifiers(event)) return;

    if (remoteViewerFile) {
      if (document.fullscreenElement && event.key === 'Escape') return;
      if (event.key === 'Escape') {
        event.preventDefault();
        onClose();
        return;
      }
      if (!remoteNestedNavigation && !isEditableShortcutTarget(event.target)) {
        if (event.key === 'ArrowLeft' || event.key.toLowerCase() === 'k') {
          event.preventDefault();
          event.stopPropagation();
          move(-1);
        } else if (event.key === 'ArrowRight' || event.key.toLowerCase() === 'j') {
          event.preventDefault();
          event.stopPropagation();
          move(1);
        }
      }
      return;
    }

    const tagInputID = `tags-${tagEditorID}`;
    if (
      (event.key === 'ArrowLeft' || event.key === 'ArrowRight')
      && event.target instanceof HTMLInputElement
      && event.target.id === tagInputID
      && event.target.value === ''
    ) {
      event.preventDefault();
      event.stopPropagation();
      event.target.blur();
      move(event.key === 'ArrowLeft' ? -1 : 1);
      return;
    }
    if (
      (event.key === '+' || event.key === '-')
      && event.target instanceof HTMLInputElement
      && event.target.id === tagInputID
      && event.target.value === ''
    ) {
      event.preventDefault();
      event.stopPropagation();
      tagMode = event.key === '-' ? 'remove' : 'add';
      return;
    }
    if (document.fullscreenElement && event.key === 'Escape') return;
    if (event.key === 'Escape') {
      event.preventDefault();
      onClose();
      return;
    }
    if (isEditableShortcutTarget(event.target)) return;
    const key = event.key.toLowerCase();
    if (key === 't' || key === 'u') {
      event.preventDefault();
      event.stopPropagation();
      focusTagInput(key === 'u' ? 'remove' : 'add');
      return;
    }
    if (event.key === 'Delete' && activeItem?.status === 'staged') {
      event.preventDefault();
      event.stopPropagation();
      removeStagedItem();
    }
  }
</script>

<svelte:window onkeydown={handleWindowKeydown} />

{#if activeItem && remoteViewerFile}
  <PreviewDialog
    file={remoteViewerFile}
    tagDraft={tagDraft}
    tagBusy={Boolean(activeItem.tagSyncPending)}
    tagError={activeItem.tagSyncError ?? ''}
    {tags}
    {onClose}
    onPrev={() => move(-1)}
    onNext={() => move(1)}
    onNestedNavigationChange={(active) => (remoteNestedNavigation = active)}
    onTagInput={(_fileID, value) => (tagDraft = value)}
    onMutateTags={(_file, operation, value) => {
      if (value == null || operation === 'set') return;
      tagMode = operation;
      commitTag(value, operation);
    }}
    onRemoveTag={(_file, tag) => removeTag(tag)}
    {onTagSearch}
    onUntrack={onRemoteUntrack}
    onDelete={onRemoteDelete}
  />
{:else if activeItem}
  <div
    bind:this={dialogElement}
    class="lightbox upload-viewer-backdrop"
    role="dialog"
    aria-modal="true"
    aria-labelledby="upload-viewer-title"
    tabindex="-1"
    onclick={(event) => { if (event.target === event.currentTarget) onClose(); }}
    onkeydown={handleBackdropKeydown}
  >
    <ViewerSidebar
      titleID="upload-viewer-title"
      eyebrow={`${scope === 'staged' ? 'Staged' : 'Queue'} · ${kindLabel}`}
      fileID={tagEditorID}
      fileName={activeItem.name}
      metadata={sidebarMetadata}
      tagValues={currentTags}
      tagCandidates={tags}
      {tagDraft}
      tagBusy={false}
      tagError={activeItem.tagSyncError ?? ''}
      {tagMode}
      closeLabel="Close upload preview"
      removeTagFrom={activeItem.name}
      {onClose}
      onTagInput={(value) => (tagDraft = value)}
      onCommitTag={commitTag}
      onRemoveTag={removeTag}
      onModeToggle={() => focusTagInput(tagMode === 'add' ? 'remove' : 'add')}
    />

    <section class="lightbox-stage upload-viewer-stage-wrap">
      {#if localMedia}
        <ViewerStage
          file={localMedia}
          {imageSource}
          initialFitMode={$runtimeConfig.viewerFitMode}
          boundActualSizeToFit={$runtimeConfig.viewerActualSizeFitCap}
          initialScaling={$runtimeConfig.viewerScaling}
          onPrev={() => move(-1)}
          onNext={() => move(1)}
          keyboardNavigation
          onFullscreenExit={onClose}
        />
      {:else if remoteLoading}
        <div class="upload-viewer-message" role="status">Loading imported media…</div>
      {:else}
        <div class="upload-viewer-message" role="alert">
          {remoteError || 'This local file format can be viewed after it is imported.'}
        </div>
      {/if}
      {#if remoteError && localMedia}
        <div class="upload-viewer-remote-warning" role="status">Imported copy unavailable: {remoteError}. Showing the local staged copy.</div>
      {/if}
    </section>

    <aside class="lightbox-rail upload-viewer-rail">
      <button class="g-btn g-btn-ghost" type="button" title="Fullscreen (F)" aria-label="Toggle fullscreen" onclick={toggleViewerFullscreen}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M8 3H3v5M16 3h5v5M8 21H3v-5M16 21h5v-5" />
        </svg>
      </button>
      <div class="rail-spacer"></div>
      {#if activeItem.status === 'staged'}
        <button class="g-btn g-btn-ghost" type="button" title="Remove from staging (Delete)" aria-label={`Remove ${activeItem.name} from staging`} onclick={removeStagedItem}>
          <Icon name="close" size={16} />
        </button>
      {/if}
    </aside>
  </div>
{/if}

<style>
  .upload-viewer-backdrop {
    position: fixed;
    z-index: 80;
  }

  .upload-viewer-stage-wrap {
    min-width: 0;
    min-height: 0;
  }

  .upload-viewer-message {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    padding: 32px;
    color: var(--text-3);
    text-align: center;
  }

  .upload-viewer-remote-warning {
    position: absolute;
    left: 50%;
    bottom: 18px;
    z-index: 4;
    max-width: min(560px, calc(100% - 36px));
    transform: translateX(-50%);
    border: 1px solid var(--border);
    border-radius: 7px;
    padding: 8px 12px;
    background: var(--bg-2);
    color: var(--text-2);
    font-family: var(--font-mono);
    font-size: 10px;
  }

  .upload-viewer-rail {
    min-width: 0;
  }
</style>
