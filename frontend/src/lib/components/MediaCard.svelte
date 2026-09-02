<script lang="ts">
  import Icon from './Icon.svelte';
  import { mediaDimensions, mediaDuration } from '$lib/utils/format';
  import type { FileItem } from '$lib/api/types';

  let {
    file,
    selected,
    selectionActive,
    onOpen,
    onToggleSelect
  } = $props<{
    file: FileItem;
    selected: boolean;
    selectionActive: boolean;
    onOpen: (file: FileItem) => void;
    onToggleSelect: (file: FileItem) => void;
  }>();

  function openOrSelect(event: MouseEvent) {
    if (selectionActive || event.shiftKey || event.metaKey || event.ctrlKey) {
      onToggleSelect(file);
      return;
    }
    onOpen(file);
  }

  function handleKeyboardAction(event: KeyboardEvent) {
    if (event.code === 'Space') {
      event.preventDefault();
      event.stopPropagation();
      onToggleSelect(file);
      return;
    }
    if (event.key === 'Enter') {
      event.preventDefault();
      event.stopPropagation();
      onOpen(file);
    }
  }
</script>

<article class={`thumb${selected ? ' is-selected' : ''}${selectionActive ? ' is-selecting' : ''}`}>
  <button
    class="thumb-open"
    type="button"
    aria-label={selectionActive ? `${selected ? 'Deselect' : 'Select'} ${file.name}` : `Preview ${file.name}`}
    onclick={openOrSelect}
    onkeydown={handleKeyboardAction}
  >
    <img src={file.media_urls.thumbnail} alt={file.name} loading="lazy" decoding="async" draggable="false" />
    <span class="thumb-overlay"></span>
    <span class="thumb-badges">
      {#if file.media_kind === 'video'}
        <span class="thumb-badge"><Icon name="play" size={9} /> {mediaDuration(file) || 'video'}</span>
      {:else if file.media_kind === 'gif'}
        <span class="thumb-badge thumb-badge-gif">GIF{mediaDuration(file) ? ` · ${mediaDuration(file)}` : ''}</span>
      {/if}
    </span>
    <span class="thumb-meta">
      <span class="thumb-meta-name">{file.name}</span>
      <span>{mediaDimensions(file)}</span>
    </span>
  </button>
  <button
    class={`thumb-checkbox${selected ? ' is-selected' : ''}`}
    type="button"
    role="checkbox"
    aria-checked={selected}
    aria-label={`${selected ? 'Deselect' : 'Select'} ${file.name}`}
    onclick={() => onToggleSelect(file)}
  >
    {#if selected}<Icon name="check" size={12} active />{/if}
  </button>
  {#if selectionActive}
    <button class="thumb-preview" type="button" aria-label={`Preview ${file.name}`} title="Preview" onclick={() => onOpen(file)}>
      <Icon name="search" size={13} />
    </button>
  {/if}
</article>

<style>
  :global(.thumb-open:focus-visible) {
    outline: 2px dashed #fff;
    outline-offset: -6px;
    box-shadow: inset 0 0 0 7px rgba(0, 0, 0, 0.72), 0 0 0 2px #000;
  }

  .thumb-preview {
    position: absolute;
    z-index: 3;
    right: 7px;
    bottom: 7px;
    width: 28px;
    height: 28px;
    display: grid;
    place-items: center;
    padding: 0;
    border: 1px solid rgba(255, 255, 255, 0.55);
    border-radius: 5px;
    background: rgba(0, 0, 0, 0.78);
    color: #fff;
    cursor: pointer;
  }

  .thumb-preview:hover,
  .thumb-preview:focus-visible {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--accent-ink);
    outline: none;
  }
</style>
