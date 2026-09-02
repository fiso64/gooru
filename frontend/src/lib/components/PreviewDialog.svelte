<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import TagEditor from './TagEditor.svelte';
  import ViewerStage from './ViewerStage.svelte';
  import { formatBytes, groupTags, mediaDimensions, mediaDuration } from '$lib/utils/format';
  import { claimFocus } from '$lib/utils/focus';
  import { hasCommandModifier, isEditableShortcutTarget } from '$lib/utils/keyboard';
  import { canUseOriginalInViewer, viewerImageSource } from '$lib/utils/media';
  import type { FileItem } from '$lib/api/types';

  let {
    file,
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
    onDelete
  } = $props<{
    file: FileItem;
    tagDraft: string;
    tagBusy: boolean;
    tagError: string;
    tags: Array<{ name?: string; tag?: string; namespace?: string; value?: string; count?: number }>;
    onClose: () => void;
    onPrev: () => void;
    onNext: () => void;
    onTagInput: (fileID: string, value: string) => void;
    onMutateTags: (file: FileItem, operation: 'add' | 'set' | 'remove', value?: string) => void;
    onRemoveTag: (file: FileItem, tag: string) => void;
    onTagSearch: (tag: string) => void;
    onUntrack: (file: FileItem) => void;
    onDelete: (file: FileItem) => void;
  }>();

  let dialogElement = $state<HTMLDivElement | undefined>();
  let preferOriginal = $state(false);
  const originalAvailable = $derived(canUseOriginalInViewer(file));
  const imageSource = $derived(viewerImageSource(file, preferOriginal));

  onMount(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : undefined;
    return claimFocus(dialogElement, previous);
  });

  function focusTagInput() {
    document.getElementById(`tags-${file.id}`)?.focus();
  }

  function handleViewerKeydown(event: KeyboardEvent) {
    if (event.defaultPrevented || hasCommandModifier(event)) return;
    if (event.key.toLowerCase() !== 't' || isEditableShortcutTarget(event.target)) return;
    event.preventDefault();
    event.stopPropagation();
    focusTagInput();
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
>
  <aside class="lightbox-aside">
    <div class="panel-row">
      <div class="g-eyebrow g-eyebrow-accent">{file.media_kind}</div>
      <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" aria-label="Close preview" onclick={onClose}>
        <Icon name="close" size={14} />
      </button>
    </div>

    <h2 id="preview-title" class="lightbox-name">{file.name}</h2>

    <dl class="lightbox-meta">
      <dt>Path</dt><dd class="path">{file.safe_display_path}</dd>
      <dt>Size</dt><dd>{mediaDimensions(file) || file.media_type} · {formatBytes(file.size)}</dd>
      {#if mediaDuration(file)}<dt>Length</dt><dd>{mediaDuration(file)}</dd>{/if}
      <dt>Modified</dt><dd>{modifiedLabel(file.modified_time)}</dd>
      <dt>Mime</dt><dd>{file.media_type}</dd>
      <dt>Hash</dt><dd class="hash">{file.content_id}</dd>
    </dl>

    <hr class="g-divider" />

    <div class="lightbox-tags">
      <div class="lightbox-tag-group-head lightbox-tags-head">
        <span>Tags · {file.tags.length}</span>
        <span class="lightbox-tag-tools">
          <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" disabled title="Tag history coming soon"><Icon name="info" size={13} /></button>
          <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" disabled title="Tag suggestions coming soon"><Icon name="sliders" size={13} /></button>
        </span>
      </div>

      {#each groupTags(file.tags) as group}
        <div class="lightbox-tag-group">
          {#if group.namespace}
            <div class="lightbox-tag-group-head"><span>{group.namespace}</span><span>{group.tags.length}</span></div>
          {/if}
          <div class="lightbox-tag-list">
            {#each group.tags as tag}
              <span class="g-tag">
                <button class="g-tag-search" type="button" aria-label={`Search for ${tag}`} onclick={() => onTagSearch(tag)}>
                  {#if tag.includes(':')}
                    <span class="ns">{tag.split(':')[0]}:</span><span>{tag.slice(tag.indexOf(':') + 1)}</span>
                  {:else}
                    <span>{tag}</span>
                  {/if}
                </button>
                <button class="g-tag-x" type="button" aria-label={`Remove ${tag}`} disabled={tagBusy} onclick={() => onRemoveTag(file, tag)}>
                  <Icon name="close" size={11} />
                </button>
              </span>
            {/each}
          </div>
        </div>
      {/each}

      <TagEditor
        fileID={file.id}
        fileName={file.name}
        draft={tagDraft}
        busy={tagBusy}
        error={tagError}
        {tags}
        existingTags={file.tags}
        onInput={(value) => onTagInput(file.id, value)}
        onCommit={(value) => onMutateTags(file, 'add', value)}
      />
    </div>
  </aside>

  <ViewerStage {file} {imageSource} {onPrev} {onNext} />

  <aside class="lightbox-rail">
    <button class="g-btn g-btn-ghost" type="button" title="Add tag" aria-label="Add tag" onclick={focusTagInput}><Icon name="tag" size={16} /></button>
    {#if originalAvailable}
      <button
        class="g-btn g-btn-ghost"
        type="button"
        aria-label={preferOriginal ? 'Use derived preview' : 'Use original media'}
        aria-pressed={preferOriginal}
        title={preferOriginal ? 'Using original media; click to use preview' : 'Load original media'}
        onclick={() => { preferOriginal = !preferOriginal; }}
      >
        <Icon name="photo" size={16} active={preferOriginal} />
      </button>
    {/if}
    <a class="g-btn g-btn-ghost" href={file.media_urls.download || file.media_urls.content} title="Download original" aria-label={`Download ${file.name}`}>
      <Icon name="download" size={16} />
    </a>
    <a class="g-btn g-btn-ghost" href={file.media_urls.content} target="_blank" rel="noreferrer" title="Open original in new tab" aria-label={`Open original ${file.name}`}>
      <Icon name="external" size={16} />
    </a>
    <div class="rail-spacer"></div>
    <button class="g-btn g-btn-ghost" type="button" disabled title="Additional info coming soon" aria-label="Info"><Icon name="info" size={16} /></button>
    {#if file.can_delete}
      <button class="g-btn g-btn-ghost" type="button" title="Delete file from disk" aria-label={`Delete ${file.name} from disk`} onclick={() => onDelete(file)}><Icon name="trash" size={16} /></button>
    {/if}
    <button class="g-btn g-btn-ghost" type="button" title="Remove from library without deleting the file" aria-label={`Untrack ${file.name} from library`} onclick={() => onUntrack(file)}><Icon name="close" size={16} /></button>
  </aside>
</div>

<style>
  :global(.lightbox-tag-list .g-tag-search) {
    display: inline-flex;
    align-items: center;
    padding: 0;
    border: 0;
    background: transparent;
    color: inherit;
    font: inherit;
    cursor: pointer;
  }

  :global(.lightbox-tag-list .g-tag-search:focus-visible) {
    outline: 2px solid var(--accent-line);
    outline-offset: 2px;
    border-radius: 2px;
  }

  :global(.lightbox-rail .g-btn[aria-pressed='true']) {
    color: var(--accent);
    background: var(--accent-soft);
    border-color: var(--accent-line);
  }
</style>
