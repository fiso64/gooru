<script lang="ts">
  import Icon from './Icon.svelte';
  import { mediaDimensions, mediaDuration } from '$lib/utils/format';
  import type { FileItem } from '$lib/api/types';

  let {
    file,
    selected,
    onOpen,
    onToggleSelect
  } = $props<{
    file: FileItem;
    selected: boolean;
    onOpen: (file: FileItem) => void;
    onToggleSelect: (file: FileItem) => void;
  }>();
</script>

<article class:selected class="thumb">
  <button class="thumb-open" type="button" aria-label={`Preview ${file.name}`} onclick={() => onOpen(file)}>
    <img src={file.media_urls.thumbnail} alt={file.name} loading="lazy" decoding="async" draggable="false" />
    <span class="thumb-overlay"></span>
    <span class="thumb-badges">
      {#if file.media_kind === 'video'}
        <span class="thumb-badge"><Icon name="play" size={9} /> {mediaDuration(file) || 'video'}</span>
      {:else if file.media_kind === 'audio' || file.media_type.startsWith('audio/')}
        <span class="thumb-badge"><Icon name="audio" size={9} /> {mediaDuration(file) || 'audio'}</span>
      {:else if file.media_kind === 'gif'}
        <span class="thumb-badge thumb-badge-gif">GIF</span>
      {/if}
    </span>
    <span class="thumb-meta">
      <span class="thumb-meta-name">{file.name}</span>
      <span>{mediaDimensions(file)}</span>
    </span>
    {#if file.tags.length}
      <span class="thumb-tags">
        {#each file.tags.slice(0, 3) as tag}
          <span>{tag}</span>
        {/each}
      </span>
    {/if}
  </button>
  <button
    class="thumb-checkbox"
    class:selected
    type="button"
    role="checkbox"
    aria-checked={selected}
    aria-label={`${selected ? 'Deselect' : 'Select'} ${file.name}`}
    onclick={() => onToggleSelect(file)}
  >
    {#if selected}<Icon name="check" size={12} active />{/if}
  </button>
</article>
