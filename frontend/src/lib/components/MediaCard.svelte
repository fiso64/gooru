<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import { mediaDimensions, mediaDuration } from '$lib/utils/format';
  import { activateNearViewport } from '$lib/utils/viewportActivation';
  import { runtimeConfig } from '$lib/stores/runtimeConfig';
  import type { FileItem } from '$lib/api/types';

  let {
    file, cardWidth, cardHeight = cardWidth, fitMedia = false, pixelRatio, viewportRoot, selected,
    selectionActive, onOpen, onToggleSelect
  } = $props<{
    file: FileItem; cardWidth: number; cardHeight?: number; fitMedia?: boolean; pixelRatio: number; viewportRoot?: Element;
    selected: boolean; selectionActive: boolean; onOpen: (file: FileItem) => void;
    onToggleSelect: (file: FileItem, range: boolean) => void;
  }>();

  let cardHost = $state<HTMLElement | undefined>();
  let thumbnailActive = $state(false);
  const extensionLabel = $derived(fileExtension(file.name));
  const mediaWidth = $derived(file.metadata.image_width ?? file.metadata.video_width ?? 0);
  const mediaHeight = $derived(file.metadata.image_height ?? file.metadata.video_height ?? 0);
  const thumbnailSource = $derived(thumbnailURL(file.media_urls.thumbnail, $runtimeConfig.thumbnailSizes,
    cardWidth || $runtimeConfig.gridSize, cardHeight || cardWidth || $runtimeConfig.gridSize,
    pixelRatio, mediaWidth, mediaHeight, fitMedia));

  onMount(() => {
    if (!cardHost) return;
    return activateNearViewport(cardHost, viewportRoot ?? null, () => { thumbnailActive = true; });
  });

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
    let requiredMaxEdge = Math.max(renderedWidth, renderedHeight) * scaleDPR;
    if (validDimensions) {
      const requiredScale = Math.max(renderedWidth * scaleDPR / mediaWidth, renderedHeight * scaleDPR / mediaHeight);
      requiredMaxEdge = Math.max(mediaWidth, mediaHeight) * requiredScale;
    }
    const selectedSize = sizes.find((size) => size >= requiredMaxEdge) ?? sizes[sizes.length - 1];
    const separator = base.includes('?') ? '&' : '?';
    return `${base}${separator}size=${selectedSize}`;
  }

  function openOrSelect(event: MouseEvent) {
    if (selectionActive || event.shiftKey || event.metaKey || event.ctrlKey) { onToggleSelect(file, event.shiftKey); return; }
    onOpen(file);
  }

  function handleKeyboardAction(event: KeyboardEvent) {
    if (event.code === 'Space') { event.preventDefault(); event.stopPropagation(); onToggleSelect(file, false); return; }
    if (event.key === 'Enter') { event.preventDefault(); event.stopPropagation(); onOpen(file); }
  }
</script>

<article bind:this={cardHost} class={`thumb${fitMedia ? ' thumb-fit' : ''}${selected ? ' is-selected' : ''}${selectionActive ? ' is-selecting' : ''}`} style={cardHeight !== cardWidth ? `height:${cardHeight}px;aspect-ratio:auto` : ''}>
  <button class="thumb-open" type="button" aria-label={selectionActive ? `${selected ? 'Deselect' : 'Select'} ${file.name}` : `Preview ${file.name}`} onclick={openOrSelect} onkeydown={handleKeyboardAction}>
    {#if thumbnailActive}<img src={thumbnailSource} alt={file.name} decoding="async" draggable="false" />{/if}
    <span class="thumb-overlay"></span>
    <span class="thumb-badges">{#if file.media_kind === 'video'}<span class="thumb-badge"><Icon name="play" size={9} /> {mediaDuration(file) || 'video'}</span>{:else if file.media_kind === 'gif'}<span class="thumb-badge">GIF{mediaDuration(file) ? ` · ${mediaDuration(file)}` : ''}</span>{:else if file.media_kind !== 'photo' && extensionLabel}<span class="thumb-badge thumb-badge-extension">{extensionLabel}</span>{/if}</span>
    <span class="thumb-meta"><span class="thumb-meta-name">{file.name}</span><span>{mediaDimensions(file)}</span></span>
  </button>
  <button class={`thumb-checkbox${selected ? ' is-selected' : ''}`} type="button" role="checkbox" aria-checked={selected} aria-label={`${selected ? 'Deselect' : 'Select'} ${file.name}`} onclick={(event) => onToggleSelect(file, event.shiftKey)}>{#if selected}<Icon name="check" size={12} active />{/if}</button>
  {#if selectionActive}<button class="thumb-preview" type="button" aria-label={`Preview ${file.name}`} title="Preview" onclick={() => onOpen(file)}><Icon name="search" size={13} /></button>{/if}
</article>

<style>
  :global(.thumb-open:focus-visible) { outline: none; }
  :global(.thumb-open:focus-visible::after) { content: ''; position: absolute; z-index: 5; inset: 6px; border: 2px dashed #fff; border-radius: 2px; box-shadow: 0 0 0 2px #000, inset 0 0 0 1px #000; pointer-events: none; }
  .thumb-badge-extension { background: var(--accent); color: var(--accent-ink); }
  .thumb-preview { position: absolute; z-index: 3; right: 7px; bottom: 7px; width: 28px; height: 28px; display: grid; place-items: center; padding: 0; border: 1px solid rgba(255, 255, 255, 0.55); border-radius: 5px; background: rgba(0, 0, 0, 0.78); color: #fff; cursor: pointer; opacity: 0; pointer-events: none; transition: opacity .12s, background .12s, border-color .12s; }
  :global(.thumb:hover) .thumb-preview { opacity: 1; pointer-events: auto; }
  .thumb-preview:hover, .thumb-preview:focus-visible { background: var(--accent); border-color: var(--accent); color: var(--accent-ink); outline: none; }
</style>
