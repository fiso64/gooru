<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import { mediaDimensions, mediaDuration } from '$lib/utils/format';
  import { activateNearViewport } from '$lib/utils/viewportActivation';
  import { activeHoverPreviewID } from '$lib/stores/hoverPreview';
  import { runtimeConfig } from '$lib/stores/runtimeConfig';
  import { isAnimatedGif } from '$lib/utils/media';
  import type { FileItem } from '$lib/api/types';

  const hoverDwellMs = 150;

  let {
    file, cardWidth, cardHeight = cardWidth, fitMedia = false, mediaInset = 0, pixelRatio, viewportRoot, selected,
    selectionActive, onOpen, onToggleSelect, onThumbnailAspect, onGridKeydown
  } = $props<{
    file: FileItem; cardWidth: number; cardHeight?: number; fitMedia?: boolean; mediaInset?: number; pixelRatio: number; viewportRoot?: Element;
    selected: boolean; selectionActive: boolean; onOpen: (file: FileItem) => void;
    onToggleSelect: (file: FileItem, range: boolean) => void;
    onThumbnailAspect?: (fileID: string, aspect: number) => void;
    onGridKeydown?: (event: KeyboardEvent) => void;
  }>();

  let cardHost = $state<HTMLElement | undefined>();
  let thumbnailActive = $state(false);
  let previewNearViewport = $state(false);
  let reducedMotion = $state(false);
  let hoverTimer: number | undefined;
  let hoverSession = $state(0);
  let previewReady = $state(false);
  let videoProgress = $state(0);
  const extensionLabel = $derived(fileExtension(file.name));
  const mediaWidth = $derived(file.metadata.image_width ?? file.metadata.video_width ?? 0);
  const mediaHeight = $derived(file.metadata.image_height ?? file.metadata.video_height ?? 0);
  const gifFile = $derived(file.media_kind === 'gif' || isAnimatedGif(file));
  const videoFile = $derived(file.media_kind === 'video');
  const hoverEnabled = $derived((videoFile && $runtimeConfig.hoverPlayVideos) || (gifFile && $runtimeConfig.hoverPlayGifs));
  const hoverPreviewActive = $derived($activeHoverPreviewID === file.id && hoverEnabled && previewNearViewport && !reducedMotion);
  const gifPreviewSource = $derived(withHoverSession(file.media_urls.content, hoverSession));
  const thumbnailSource = $derived(thumbnailURL(file.media_urls.thumbnail, $runtimeConfig.thumbnailSizes,
    Math.max(1, (cardWidth || $runtimeConfig.gridSize) - mediaInset * 2),
    Math.max(1, (cardHeight || cardWidth || $runtimeConfig.gridSize) - mediaInset * 2),
    pixelRatio, mediaWidth, mediaHeight, fitMedia));

  onMount(() => {
    if (!cardHost) return;
    const stopThumbnailActivation = activateNearViewport(cardHost, viewportRoot ?? null, () => { thumbnailActive = true; });
    const motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
    const updateMotion = () => {
      reducedMotion = motionQuery.matches;
      if (reducedMotion) stopHoverPreview();
    };
    updateMotion();
    motionQuery.addEventListener('change', updateMotion);

    const previewObserver = typeof IntersectionObserver === 'undefined'
      ? undefined
      : new IntersectionObserver((entries) => {
          previewNearViewport = entries.some((entry) => entry.isIntersecting);
          if (!previewNearViewport) stopHoverPreview();
        }, { root: viewportRoot ?? null, rootMargin: '96px 0px' });
    if (previewObserver) previewObserver.observe(cardHost);
    else previewNearViewport = true;

    return () => {
      stopThumbnailActivation();
      if (hoverTimer !== undefined) window.clearTimeout(hoverTimer);
      if ($activeHoverPreviewID === file.id) activeHoverPreviewID.set('');
      motionQuery.removeEventListener('change', updateMotion);
      previewObserver?.disconnect();
    };
  });

  function startHoverPreview() {
    if (!hoverEnabled || reducedMotion || !previewNearViewport || !file.media_urls.content) return;
    if (hoverTimer !== undefined) window.clearTimeout(hoverTimer);
    hoverTimer = window.setTimeout(() => {
      hoverTimer = undefined;
      if (hoverEnabled && !reducedMotion && previewNearViewport) {
        previewReady = false;
        videoProgress = 0;
        hoverSession += 1;
        activeHoverPreviewID.set(file.id);
      }
    }, hoverDwellMs);
  }

  function stopHoverPreview() {
    if (hoverTimer !== undefined) {
      window.clearTimeout(hoverTimer);
      hoverTimer = undefined;
    }
    previewReady = false;
    videoProgress = 0;
    if ($activeHoverPreviewID === file.id) activeHoverPreviewID.set('');
  }

  function markPreviewReady() {
    previewReady = true;
  }

  function reportThumbnailAspect(event: Event) {
    const image = event.currentTarget as HTMLImageElement;
    const hasSourceDimensions = Number.isFinite(mediaWidth) && Number.isFinite(mediaHeight) && mediaWidth > 0 && mediaHeight > 0;
    if (hasSourceDimensions || !onThumbnailAspect || image.naturalWidth <= 0 || image.naturalHeight <= 0) return;
    onThumbnailAspect(file.id, image.naturalWidth / image.naturalHeight);
  }

  function updateVideoProgress(event: Event) {
    const video = event.currentTarget as HTMLVideoElement;
    videoProgress = Number.isFinite(video.duration) && video.duration > 0 ? Math.min(1, Math.max(0, video.currentTime / video.duration)) : 0;
  }

  function withHoverSession(source: string, session: number) {
    if (!source) return source;
    const base = source.split('#', 1)[0];
    const separator = base.includes('?') ? '&' : '?';
    return `${base}${separator}gooru_hover_session=${session}#gooru-hover-${session}`;
  }

  function fileExtension(name: string) {
    const baseName = name.split(/[\\/]/).pop() ?? name;
    const dot = baseName.lastIndexOf('.');
    if (dot <= 0 || dot === baseName.length - 1) return '';
    return baseName.slice(dot + 1).toUpperCase();
  }

  function thumbnailURL(base: string, sizes: number[], cssWidth: number, cssHeight: number, dpr: number, mediaWidth: number, mediaHeight: number, contain: boolean) {
    if (!sizes.length) return base;
    const scaleDPR = Math.max(1, dpr);
    const validDimensions = mediaWidth > 0 && mediaHeight > 0 && Number.isFinite(mediaWidth) && Number.isFinite(mediaHeight);
    let renderedWidth = Math.max(1, cssWidth);
    let renderedHeight = Math.max(1, cssHeight);
    if (contain && validDimensions) {
      const mediaAspect = mediaWidth / mediaHeight;
      const boxAspect = renderedWidth / renderedHeight;
      if (mediaAspect >= boxAspect) renderedHeight = renderedWidth / mediaAspect;
      else renderedWidth = renderedHeight * mediaAspect;
    }
    let requiredShortEdge = Math.max(renderedWidth, renderedHeight) * scaleDPR;
    if (validDimensions) {
      const requiredScale = Math.max(renderedWidth * scaleDPR / mediaWidth, renderedHeight * scaleDPR / mediaHeight);
      requiredShortEdge = Math.min(mediaWidth, mediaHeight) * requiredScale;
    }
    const selectedSize = sizes.find((size) => size >= requiredShortEdge) ?? sizes[sizes.length - 1];
    const separator = base.includes('?') ? '&' : '?';
    return `${base}${separator}size=${selectedSize}`;
  }

  function openOrSelect(event: MouseEvent) {
    if (selectionActive || event.shiftKey || event.metaKey || event.ctrlKey) { onToggleSelect(file, event.shiftKey); return; }
    onOpen(file);
  }

  function handleKeyboardAction(event: KeyboardEvent) {
    if (event.code === 'Space') { event.preventDefault(); event.stopPropagation(); onToggleSelect(file, false); return; }
    if (event.key === 'Enter' && !event.altKey) { event.preventDefault(); event.stopPropagation(); onOpen(file); }
  }

  function handleKeydown(event: KeyboardEvent) {
    handleKeyboardAction(event);
    if (!event.cancelBubble) onGridKeydown?.(event);
  }
</script>

<article bind:this={cardHost} class={`thumb${fitMedia ? ' thumb-fit' : ''}${selected ? ' is-selected' : ''}${selectionActive ? ' is-selecting' : ''}`} style={`--thumb-media-inset:${mediaInset}px;${cardHeight !== cardWidth ? `height:${cardHeight}px;aspect-ratio:auto` : ''}`} onpointerenter={startHoverPreview} onpointerleave={stopHoverPreview}>
  <button class="thumb-open" type="button" data-file-id={file.id} aria-label={selectionActive ? `${selected ? 'Deselect' : 'Select'} ${file.name}` : `Preview ${file.name}`} onclick={openOrSelect} onkeydown={handleKeydown}>
    {#if thumbnailActive}<img class:preview-covered={hoverPreviewActive && previewReady} src={thumbnailSource} alt={file.name} decoding="async" draggable="false" onload={reportThumbnailAspect} />{/if}
    {#if hoverPreviewActive && videoFile}
      <video class:contain-preview={fitMedia} class:is-ready={previewReady} class="hover-preview-media" data-testid="hover-video-preview" src={file.media_urls.content} muted autoplay loop playsinline preload="metadata" onloadeddata={markPreviewReady} ontimeupdate={updateVideoProgress} ondurationchange={updateVideoProgress}></video>
      <span class:is-ready={previewReady} class="hover-video-progress" data-testid="hover-video-progress" aria-hidden="true"><span style={`transform:scaleX(${videoProgress})`}></span></span>
    {:else if hoverPreviewActive && gifFile}
      <img class:contain-preview={fitMedia} class:is-ready={previewReady} class="hover-preview-media" data-testid="hover-gif-preview" src={gifPreviewSource} onload={markPreviewReady} alt="" draggable="false" />
    {/if}
    <span class="thumb-overlay"></span>
    <span class="thumb-badges">{#if file.media_kind === 'video'}<span class="thumb-badge"><Icon name="play" size={9} /> {mediaDuration(file) || 'video'}</span>{:else if file.media_kind === 'gif'}<span class="thumb-badge">GIF{mediaDuration(file) ? ` · ${mediaDuration(file)}` : ''}</span>{:else if file.media_kind !== 'photo' && extensionLabel}<span class="thumb-badge thumb-badge-extension">{extensionLabel}</span>{/if}</span>
    <span class="thumb-meta"><span class="thumb-meta-name">{file.name}</span><span>{mediaDimensions(file)}</span></span>
  </button>
  <button class={`thumb-checkbox${selected ? ' is-selected' : ''}`} type="button" role="checkbox" aria-checked={selected} aria-label={`${selected ? 'Deselect' : 'Select'} ${file.name}`} onclick={(event) => onToggleSelect(file, event.shiftKey)}>{#if selected}<Icon name="check" size={12} active />{/if}</button>
  {#if selectionActive}<button class="thumb-preview" type="button" aria-label={`Preview ${file.name}`} title="Preview" onclick={() => onOpen(file)}><Icon name="search" size={13} /></button>{/if}
</article>

<style>
  :global(.thumb-open:focus-visible) { outline: none; }
  :global(.thumb-open:focus-visible::after) { content: ''; position: absolute; z-index: 6; inset: 6px; border: 2px dashed #fff; border-radius: 2px; box-shadow: 0 0 0 2px #000, inset 0 0 0 1px #000; pointer-events: none; }
  :global(.thumb-open > img.preview-covered) { opacity: 0; }
  .hover-preview-media { position: absolute; z-index: 1; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; pointer-events: none; }
  .hover-preview-media.is-ready { opacity: 1; }
  .hover-preview-media.contain-preview { object-fit: contain; }
  .thumb-overlay { z-index: 2; }
  .thumb-badges, .thumb-meta { z-index: 3; }
  .hover-video-progress { position: absolute; z-index: 3; left: 7px; right: 7px; bottom: 5px; height: 2px; border-radius: 2px; overflow: hidden; background: rgba(255, 255, 255, 0.28); opacity: 0; pointer-events: none; }
  .hover-video-progress.is-ready { opacity: 1; }
  .hover-video-progress > span { display: block; width: 100%; height: 100%; transform-origin: left center; background: rgba(255, 255, 255, 0.9); }
  .thumb-checkbox { z-index: 4; }
  :global(.thumb.is-selected)::after { z-index: 5; }
  .thumb-badge-extension { background: var(--accent); color: var(--accent-ink); }
  .thumb-preview { position: absolute; z-index: 4; right: 7px; bottom: 7px; width: 28px; height: 28px; display: grid; place-items: center; padding: 0; border: 1px solid rgba(255, 255, 255, 0.55); border-radius: 5px; background: rgba(0, 0, 0, 0.78); color: #fff; cursor: pointer; opacity: 0; pointer-events: none; transition: opacity .12s, background .12s, border-color .12s; }
  :global(.thumb:hover) .thumb-preview { opacity: 1; pointer-events: auto; }
  .thumb-preview:hover, .thumb-preview:focus-visible { background: var(--accent); border-color: var(--accent); color: var(--accent-ink); outline: none; }
</style>