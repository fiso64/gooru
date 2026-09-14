<script lang="ts">
  import Icon from './Icon.svelte';
  import TagEditor from './TagEditor.svelte';
  import ViewerStage from './ViewerStage.svelte';
  import { ApiClient } from '$lib/api/client';
  import type { FileItem } from '$lib/api/types';
  import { authState } from '$lib/stores/auth';
  import { runtimeConfig } from '$lib/stores/runtimeConfig';
  import type { UploadItem } from '$lib/state/uploadItems';
  import { uploadItemHasLocalViewer, uploadViewerNeighborIndex, type UploadViewerScope } from '$lib/state/uploadViewer';
  import { errorMessage, formatBytes, groupTags, parseTags } from '$lib/utils/format';
  import { hasCommandModifier, isEditableShortcutTarget } from '$lib/utils/keyboard';
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
    onItemRemoteTagsLoaded
  } = $props<{
    uploadItems: UploadItem[];
    activeIndex: number;
    scope: UploadViewerScope;
    tags: TagCandidate[];
    onIndex: (index: number) => void;
    onClose: () => void;
    onItemTagsInput: (index: number, tags: string[]) => void;
    onItemRemoteTagsLoaded: (index: number, tags: string[]) => void;
  }>();

  let localURL = $state('');
  let remoteFile = $state<FileItem | undefined>();
  let remoteLoading = $state(false);
  let remoteError = $state('');
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
  const viewerFile = $derived<ViewerStageMedia | undefined>(remoteFile ?? localMedia);
  const imageSource = $derived(viewerFile ? viewerImageSource(viewerFile, false) : '');
  const currentTags = $derived(tagOverride ?? activeItem?.tags ?? remoteFile?.tags ?? []);
  const tagGroups = $derived(groupTags(currentTags));
  const hasTagNamespaces = $derived(tagGroups.some((group) => Boolean(group.namespace)));
  const kindLabel = $derived(remoteFile?.media_kind ?? localMedia?.media_kind ?? 'media');
  const tagEditorID = $derived(`upload-${activeIndex}`);

  $effect(() => {
    if (activeIndex === observedIndex) return;
    observedIndex = activeIndex;
    tagDraft = '';
    tagMode = 'add';
    tagOverride = undefined;
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

    const controller = new AbortController();
    const client = new ApiClient($authState.csrfToken);
    void client.getFile(remoteFileID, controller.signal)
      .then((file) => {
        if (controller.signal.aborted) return;
        remoteFile = file;
        onItemRemoteTagsLoaded(activeIndex, file.tags ?? []);
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

  $effect(() => {
    if (!activeItem) onClose();
  });

  function move(delta: number) {
    const nextIndex = uploadViewerNeighborIndex(uploadItems, activeIndex, scope, delta);
    if (nextIndex != null && nextIndex !== activeIndex) onIndex(nextIndex);
  }

  function setDesiredTags(nextTags: string[]) {
    tagOverride = nextTags;
    onItemTagsInput(activeIndex, nextTags);
  }

  function commitTag(value: string) {
    const changed = parseTags(value);
    if (!changed.length) return;
    if (tagMode === 'remove') {
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

  function handleWindowKeydown(event: KeyboardEvent) {
    if (event.defaultPrevented || hasCommandModifier(event)) return;
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
    }
  }
</script>

<svelte:window onkeydown={handleWindowKeydown} />

{#if activeItem}
  <div
    class="lightbox upload-viewer-backdrop"
    role="dialog"
    aria-modal="true"
    aria-labelledby="upload-viewer-title"
    tabindex="-1"
    onclick={(event) => { if (event.target === event.currentTarget) onClose(); }}
  >
    <aside class="lightbox-aside">
      <div class="panel-row">
        <div class="g-eyebrow g-eyebrow-accent">{scope === 'staged' ? 'Staged' : 'Queue'} · {kindLabel}</div>
        <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" aria-label="Close upload preview" onclick={onClose}>
          <Icon name="close" size={14} />
        </button>
      </div>

      <h2 id="upload-viewer-title" class="lightbox-name">{activeItem.name}</h2>
      <dl class="lightbox-meta">
        {#if remoteFile?.safe_display_path}<dt>Path</dt><dd class="path">{remoteFile.safe_display_path}</dd>{/if}
        <dt>Size</dt><dd>{formatBytes(activeItem.size)}</dd>
        <dt>Status</dt><dd>{activeItem.status.replace(/_/g, ' ')}</dd>
        <dt>Mime</dt><dd>{activeItem.type || remoteFile?.media_type || 'unknown'}</dd>
        {#if activeItem.batchID != null}<dt>Batch</dt><dd>{activeItem.batchID}</dd>{/if}
        {#if remoteFile?.content_id}<dt>Id</dt><dd class="hash">{remoteFile.content_id}</dd>{/if}
      </dl>

      <hr class="g-divider" />

      <div class="lightbox-tags">
        <div class="lightbox-tag-group-head lightbox-tags-head">
          <span>Tags · {currentTags.length}</span>
          <span class="lightbox-tag-tools">
            <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" disabled title="Tag history coming soon"><Icon name="info" size={13} /></button>
            <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" disabled title="Tag suggestions coming soon"><Icon name="sliders" size={13} /></button>
          </span>
        </div>

        {#each tagGroups as group (group.namespace)}
          <div class="lightbox-tag-group">
            {#if group.namespace || hasTagNamespaces}
              <div class="lightbox-tag-group-head"><span>{group.namespace || 'OTHER'}</span><span>{group.tags.length}</span></div>
            {/if}
            <div class="lightbox-tag-list">
              {#each group.tags as tag}
                <span class="g-tag">
                  {#if tag.includes(':')}
                    <span class="ns">{tag.split(':')[0]}:</span><span>{tag.slice(tag.indexOf(':') + 1)}</span>
                  {:else}
                    <span>{tag}</span>
                  {/if}
                  <button class="g-tag-x" type="button" aria-label={`Remove ${tag} from ${activeItem.name}`} onclick={() => removeTag(tag)}>
                    <Icon name="close" size={11} />
                  </button>
                </span>
              {/each}
            </div>
          </div>
        {/each}

        <TagEditor
          fileID={tagEditorID}
          fileName={activeItem.name}
          draft={tagDraft}
          busy={false}
          error={activeItem.tagSyncError ?? ''}
          {tags}
          existingTags={currentTags}
          mode={tagMode}
          onInput={(value) => (tagDraft = value)}
          onCommit={commitTag}
          onModeToggle={() => focusTagInput(tagMode === 'add' ? 'remove' : 'add')}
        />
        {#if activeItem.tagSyncPending}
          <div class="upload-viewer-sync">{activeItem.remoteFileID ? 'Saving tag changes…' : 'Tag changes will apply when import completes'}</div>
        {/if}
      </div>
    </aside>

    <section class="lightbox-stage upload-viewer-stage-wrap">
      {#if viewerFile}
        <ViewerStage
          file={viewerFile}
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

    <aside class="lightbox-rail upload-viewer-rail" aria-hidden="true"></aside>
  </div>
{/if}

<style>
  .upload-viewer-backdrop {
    position: fixed;
    z-index: 80;
  }

  .upload-viewer-sync {
    color: var(--text-3);
    font-family: var(--font-mono);
    font-size: 10px;
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
