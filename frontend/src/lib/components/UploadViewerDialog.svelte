<script lang="ts">
  import Icon from './Icon.svelte';
  import TagAutocompleteInput from './TagAutocompleteInput.svelte';
  import ViewerStage from './ViewerStage.svelte';
  import { ApiClient } from '$lib/api/client';
  import type { FileItem } from '$lib/api/types';
  import { authState } from '$lib/stores/auth';
  import { runtimeConfig } from '$lib/stores/runtimeConfig';
  import type { UploadItem } from '$lib/state/uploadItems';
  import { uploadItemHasLocalViewer, uploadViewerNeighborIndex, type UploadViewerScope } from '$lib/state/uploadViewer';
  import { errorMessage, formatBytes, parseTags } from '$lib/utils/format';
  import { viewerImageSource, type ViewerStageMedia } from '$lib/utils/media';
  import type { TagCandidate } from '$lib/utils/tagSuggestions';

  let {
    uploadItems,
    activeIndex,
    scope,
    tags,
    onIndex,
    onClose,
    onItemTagsInput
  } = $props<{
    uploadItems: UploadItem[];
    activeIndex: number;
    scope: UploadViewerScope;
    tags: TagCandidate[];
    onIndex: (index: number) => void;
    onClose: () => void;
    onItemTagsInput: (index: number, tags: string[]) => void;
  }>();

  let localURL = $state('');
  let remoteFile = $state<FileItem | undefined>();
  let remoteLoading = $state(false);
  let remoteError = $state('');
  let tagDraft = $state('');
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
  const viewerFile = $derived<ViewerStageMedia | undefined>(remoteFile ?? localMedia);
  const imageSource = $derived(viewerFile ? viewerImageSource(viewerFile, false) : '');
  const currentTags = $derived(tagOverride ?? remoteFile?.tags ?? activeItem?.tags ?? []);
  const kindLabel = $derived(remoteFile?.media_kind ?? localMedia?.media_kind ?? 'media');

  $effect(() => {
    if (activeIndex === observedIndex) return;
    observedIndex = activeIndex;
    tagDraft = '';
    tagOverride = undefined;
  });

  $effect(() => {
    const file = activeItem?.previewFile;
    if (!file || !uploadItemHasLocalViewer(activeItem)) {
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

    const controller = new AbortController();
    const client = new ApiClient($authState.csrfToken);
    void client.getFile(remoteFileID, controller.signal)
      .then((file) => {
        if (!controller.signal.aborted) remoteFile = file;
      })
      .catch((error) => {
        if (!controller.signal.aborted) remoteError = errorMessage(error);
      })
      .finally(() => {
        if (!controller.signal.aborted) remoteLoading = false;
      });
    return () => controller.abort();
  });

  $effect(() => {
    if (!activeItem) onClose();
  });

  function move(delta: number) {
    const nextIndex = uploadViewerNeighborIndex(uploadItems, activeIndex, scope, delta);
    if (nextIndex != null && nextIndex !== activeIndex) onIndex(nextIndex);
  }

  function commitTag(value: string) {
    const additions = parseTags(value);
    if (!additions.length) return;
    const nextTags = Array.from(new Set([...currentTags, ...additions]));
    tagOverride = nextTags;
    tagDraft = '';
    onItemTagsInput(activeIndex, nextTags);
  }

  function removeTag(tag: string) {
    const nextTags = currentTags.filter((candidate) => candidate !== tag);
    tagOverride = nextTags;
    onItemTagsInput(activeIndex, nextTags);
  }

  function handleWindowKeydown(event: KeyboardEvent) {
    if (event.defaultPrevented || event.key !== 'Escape') return;
    event.preventDefault();
    onClose();
  }
</script>

<svelte:window onkeydown={handleWindowKeydown} />

{#if activeItem}
  <div
    class="upload-viewer-backdrop"
    role="dialog"
    aria-modal="true"
    aria-labelledby="upload-viewer-title"
    tabindex="-1"
    onclick={(event) => { if (event.target === event.currentTarget) onClose(); }}
  >
    <aside class="upload-viewer-aside">
      <div class="upload-viewer-head">
        <div class="g-eyebrow g-eyebrow-accent">{scope === 'staged' ? 'Staged' : 'Queue'} · {kindLabel}</div>
        <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" aria-label="Close upload preview" onclick={onClose}>
          <Icon name="close" size={14} />
        </button>
      </div>

      <h2 id="upload-viewer-title" class="upload-viewer-name">{activeItem.name}</h2>
      <dl class="upload-viewer-meta">
        <dt>Size</dt><dd>{formatBytes(activeItem.size)}</dd>
        <dt>Status</dt><dd>{activeItem.status.replace(/_/g, ' ')}</dd>
        <dt>Mime</dt><dd>{activeItem.type || remoteFile?.media_type || 'unknown'}</dd>
        {#if activeItem.batchID != null}<dt>Batch</dt><dd>{activeItem.batchID}</dd>{/if}
      </dl>

      <hr class="g-divider" />

      <div class="upload-viewer-tags">
        <div class="g-eyebrow">Tags · {currentTags.length}</div>
        <div class="upload-viewer-tag-list">
          {#each currentTags as tag}
            {@const separator = tag.indexOf(':')}
            <span class="g-tag">
              {#if separator > 0}
                <span class="g-tag-ns">{tag.slice(0, separator)}:</span><span>{tag.slice(separator + 1)}</span>
              {:else}
                <span>{tag}</span>
              {/if}
              <button class="g-tag-x" type="button" aria-label={`Remove ${tag} from ${activeItem.name}`} onclick={() => removeTag(tag)}>
                <Icon name="close" size={11} />
              </button>
            </span>
          {/each}
        </div>
        <div class="upload-viewer-tag-input">
          <TagAutocompleteInput
            value={tagDraft}
            {tags}
            existing={currentTags}
            placeholder="add tag"
            ariaLabel={`Add tag to ${activeItem.name}`}
            onInput={(value) => (tagDraft = value)}
            onCommit={commitTag}
            onRemoveLast={removeTag}
          />
        </div>
        {#if activeItem.tagSyncPending}
          <div class="upload-viewer-sync">{activeItem.remoteFileID ? 'Saving tag changes…' : 'Tag changes will apply when import completes'}</div>
        {/if}
        {#if activeItem.tagSyncError}<div class="upload-viewer-error" role="alert">{activeItem.tagSyncError}</div>{/if}
      </div>
    </aside>

    <section class="upload-viewer-stage-wrap">
      {#if viewerFile}
        <ViewerStage
          file={viewerFile}
          {imageSource}
          initialFitMode={$runtimeConfig.viewerFitMode}
          boundActualSizeToFit={$runtimeConfig.viewerActualSizeFitCap}
          initialScaling={$runtimeConfig.viewerScaling}
          onPrev={() => move(-1)}
          onNext={() => move(1)}
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
  </div>
{/if}

<style>
  .upload-viewer-backdrop {
    position: fixed;
    inset: 0;
    z-index: 80;
    display: grid;
    grid-template-columns: minmax(260px, 340px) 1fr;
    background: color-mix(in srgb, var(--bg-1) 88%, transparent);
    backdrop-filter: blur(10px);
  }

  .upload-viewer-aside {
    min-width: 0;
    overflow: auto;
    padding: 18px;
    border-right: 1px solid var(--border);
    background: var(--bg-2);
  }

  .upload-viewer-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  .upload-viewer-name {
    margin: 12px 0 18px;
    overflow-wrap: anywhere;
    font-size: 18px;
  }

  .upload-viewer-meta {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: 7px 12px;
    margin: 0;
    font-family: var(--font-mono);
    font-size: 11px;
  }

  .upload-viewer-meta dt { color: var(--text-3); }
  .upload-viewer-meta dd { margin: 0; min-width: 0; overflow-wrap: anywhere; text-align: right; }

  .upload-viewer-tags { display: grid; gap: 10px; }
  .upload-viewer-tag-list { display: flex; flex-wrap: wrap; gap: 5px; }
  .upload-viewer-tag-input { min-height: 34px; }
  .upload-viewer-sync { color: var(--text-3); font-family: var(--font-mono); font-size: 10px; }
  .upload-viewer-error { color: var(--danger); font-family: var(--font-mono); font-size: 11px; }

  .upload-viewer-stage-wrap {
    position: relative;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
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

  @media (max-width: 760px) {
    .upload-viewer-backdrop { grid-template-columns: 1fr; grid-template-rows: auto 1fr; }
    .upload-viewer-aside { max-height: 42vh; border-right: 0; border-bottom: 1px solid var(--border); }
  }
</style>
