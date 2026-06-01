<script lang="ts">
  import Icon from './Icon.svelte';
  import TagEditor from './TagEditor.svelte';
  import { formatBytes, groupTags, mediaDimensions, mediaDuration, parseTags } from '$lib/utils/format';
  import type { FileItem } from '$lib/api/types';

  let {
    file,
    tagDraft,
    tagBusy,
    tagError,
    onClose,
    onPrev,
    onNext,
    onTagInput,
    onMutateTags
  } = $props<{
    file: FileItem;
    tagDraft: string;
    tagBusy: boolean;
    tagError: string;
    onClose: () => void;
    onPrev: () => void;
    onNext: () => void;
    onTagInput: (fileID: string, value: string) => void;
    onMutateTags: (file: FileItem, operation: 'add' | 'set' | 'remove') => void;
  }>();
</script>

<div class="lightbox" role="dialog" aria-modal="true" aria-labelledby="preview-title">
  <aside class="lightbox-aside">
    <div class="panel-row">
      <div class="g-eyebrow g-eyebrow-accent">{file.media_kind}</div>
      <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" aria-label="Close preview" onclick={onClose}>
        <Icon name="close" size={14} />
      </button>
    </div>

    <h2 id="preview-title" class="lightbox-name">{file.name}</h2>

    <dl class="lightbox-meta">
      <dt>Path</dt><dd>{file.safe_display_path}</dd>
      <dt>Size</dt><dd>{mediaDimensions(file) || file.media_type} · {formatBytes(file.size)}</dd>
      {#if mediaDuration(file)}<dt>Length</dt><dd>{mediaDuration(file)}</dd>{/if}
      <dt>Modified</dt><dd>{new Date(file.modified_time).toLocaleString()}</dd>
      <dt>Mime</dt><dd>{file.media_type}</dd>
      <dt>Hash</dt><dd class="hash">{file.content_id}</dd>
    </dl>

    <hr class="g-divider" />

    <div class="lightbox-tags">
      <div class="lightbox-tag-group-head">
        <span>Tags · {file.tags.length}</span>
      </div>
      {#each groupTags(file.tags) as group}
        <div class="lightbox-tag-group">
          {#if group.namespace}<div class="lightbox-tag-group-head"><span>{group.namespace}</span><span>{group.tags.length}</span></div>{/if}
          <div class="lightbox-tag-list">
            {#each group.tags as tag}
              <span class="g-tag">
                {#if tag.includes(':')}<span class="ns">{tag.split(':')[0]}:</span><span>{tag.slice(tag.indexOf(':') + 1)}</span>{:else}{tag}{/if}
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
        canSubmit={Boolean(parseTags(tagDraft).length)}
        onInput={(value) => onTagInput(file.id, value)}
        onMutate={(operation) => onMutateTags(file, operation)}
      />
    </div>
  </aside>

  <div class="lightbox-stage">
    {#if file.media_kind === 'video'}
      <!-- svelte-ignore a11y_media_has_caption -->
      <video src={file.media_urls.content} poster={file.media_urls.preview} controls preload="metadata"></video>
    {:else if file.media_kind === 'audio' || file.media_type.startsWith('audio/')}
      <div class="audio-stage">
        <div class="audio-art"><Icon name="audio" size={42} /></div>
        <audio src={file.media_urls.content} controls preload="metadata"></audio>
      </div>
    {:else}
      <img src={file.media_urls.preview} alt={file.name} />
    {/if}
    <button class="lightbox-nav-arrow prev" type="button" title="Previous" aria-label="Previous file" onclick={onPrev}><Icon name="close" size={18} /></button>
    <button class="lightbox-nav-arrow next" type="button" title="Next" aria-label="Next file" onclick={onNext}><Icon name="close" size={18} /></button>
  </div>

  <aside class="lightbox-rail">
    <a class="g-btn g-btn-ghost" href={file.media_urls.download || file.media_urls.content} title="Download original" aria-label={`Download ${file.name}`}>
      <Icon name="download" size={16} />
    </a>
    <a class="g-btn g-btn-ghost" href={file.media_urls.content} target="_blank" rel="noreferrer" title="Open original in new tab" aria-label={`Open original ${file.name}`}>
      <Icon name="external" size={16} />
    </a>
    <div class="rail-spacer"></div>
    <button class="g-btn g-btn-ghost" type="button" title="Info"><Icon name="info" size={16} /></button>
  </aside>
</div>
