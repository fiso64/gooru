<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import TagEditor from './TagEditor.svelte';
  import { formatBytes, groupTags, mediaDimensions, mediaDuration, parseTags } from '$lib/utils/format';
  import { claimFocus } from '$lib/utils/focus';
  import { hasCommandModifier, isInteractiveShortcutTarget } from '$lib/utils/keyboard';
  import { canUseOriginalInViewer, preserveNativeViewerSize, viewerImageSource } from '$lib/utils/media';
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
    onUntrack
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
  }>();

  let dialogElement = $state<HTMLDivElement | undefined>();
  let videoElement = $state<HTMLVideoElement | undefined>();
  let audioElement = $state<HTMLAudioElement | undefined>();
  let videoPaused = $state(true);
  let videoTime = $state(0);
  let videoLength = $state(0);
  let preferOriginal = $state(false);
  const videoProgress = $derived(videoLength > 0 ? Math.min(100, Math.max(0, (videoTime / videoLength) * 100)) : 0);
  const originalAvailable = $derived(canUseOriginalInViewer(file));
  const imageSource = $derived(viewerImageSource(file, preferOriginal));
  const nativeImageSize = $derived(preserveNativeViewerSize(file));

  onMount(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : undefined;
    return claimFocus(dialogElement, previous);
  });

  $effect(() => {
    file.id;
    videoPaused = true;
    videoTime = 0;
    videoLength = 0;
  });

  function focusTagInput() {
    document.getElementById(`tags-${file.id}`)?.focus();
  }

  async function togglePlayback() {
    const media = videoElement ?? audioElement;
    if (!media) return;
    if (media.paused) await media.play();
    else media.pause();
  }

  function handlePlaybackKeydown(event: KeyboardEvent) {
    if (event.defaultPrevented || hasCommandModifier(event) || isInteractiveShortcutTarget(event.target)) return;
    if (event.code !== 'Space') return;
    if (!videoElement && !audioElement) return;
    event.preventDefault();
    void togglePlayback();
  }

  function syncVideo() {
    if (!videoElement) return;
    videoPaused = videoElement.paused;
    videoTime = videoElement.currentTime;
    videoLength = Number.isFinite(videoElement.duration) ? videoElement.duration : 0;
  }

  function seekVideo(event: MouseEvent) {
    const video = videoElement;
    if (!video || !videoLength) return;
    const button = event.currentTarget;
    if (!(button instanceof HTMLElement)) return;
    const rect = button.getBoundingClientRect();
    if (!rect.width) return;
    video.currentTime = Math.max(0, Math.min(videoLength, ((event.clientX - rect.left) / rect.width) * videoLength));
    syncVideo();
  }

  function clock(seconds: number) {
    if (!Number.isFinite(seconds) || seconds < 0) return '0:00';
    const whole = Math.floor(seconds);
    return `${Math.floor(whole / 60)}:${String(whole % 60).padStart(2, '0')}`;
  }

  function modifiedLabel(value: string) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;
    return date.toLocaleString('en-GB', { year: 'numeric', month: 'short', day: '2-digit', hour: '2-digit', minute: '2-digit' });
  }
</script>

<svelte:window onkeydown={handlePlaybackKeydown} />

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

  <div class="lightbox-stage">
    {#if file.media_kind === 'video'}
      <!-- svelte-ignore a11y_media_has_caption -->
      <video
        bind:this={videoElement}
        src={file.media_urls.content}
        poster={file.media_urls.preview}
        preload="metadata"
        autoplay
        loop
        onclick={() => void togglePlayback()}
        onloadedmetadata={syncVideo}
        ontimeupdate={syncVideo}
        onplay={syncVideo}
        onpause={syncVideo}
        onended={syncVideo}
      ></video>

      <div class="lightbox-video-controls">
        <button class="video-play" type="button" aria-label={videoPaused ? 'Play video' : 'Pause video'} onclick={() => void togglePlayback()}>
          <Icon name={videoPaused ? 'play' : 'pause'} size={14} />
        </button>
        <span class="video-time">{clock(videoTime)}</span>
        <button class="video-progress" type="button" aria-label="Seek video" onclick={seekVideo}>
          <span style={`width: ${videoProgress}%`}></span>
        </button>
        <span class="video-time">{videoLength ? clock(videoLength) : (mediaDuration(file) || '0:00')}</span>
      </div>
    {:else if file.media_kind === 'audio' || file.media_type.startsWith('audio/')}
      <div class="audio-stage">
        <div class="audio-art"><Icon name="audio" size={42} /></div>
        <audio bind:this={audioElement} src={file.media_urls.content} controls preload="metadata"></audio>
      </div>
    {:else}
      <img class:native-size={nativeImageSize} src={imageSource} alt={file.name} />
    {/if}

    <button class="lightbox-nav-arrow prev" type="button" title="Previous (←)" aria-label="Previous file" onclick={onPrev}><Icon name="chev_left" size={20} /></button>
    <button class="lightbox-nav-arrow next" type="button" title="Next (→)" aria-label="Next file" onclick={onNext}><Icon name="chev_right" size={20} /></button>
  </div>

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
    <button class="g-btn g-btn-ghost" type="button" title="Remove from library" aria-label={`Remove ${file.name} from library`} onclick={() => onUntrack(file)}><Icon name="trash" size={16} /></button>
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

  :global(.lightbox-stage img.native-size) {
    inset: 50% auto auto 50%;
    width: auto;
    height: auto;
    max-width: calc(100% - 72px);
    max-height: calc(100% - 72px);
    transform: translate(-50%, -50%);
  }

  :global(.lightbox-rail .g-btn[aria-pressed='true']) {
    color: var(--accent);
    background: var(--accent-soft);
    border-color: var(--accent-line);
  }
</style>
