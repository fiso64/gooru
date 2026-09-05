<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import type { UploadItem } from '$lib/state/uploadItems';

  let { file, item } = $props<{ file?: File; item: UploadItem }>();
  let host = $state<HTMLDivElement | undefined>();
  let visible = $state(false);
  let previewURL = $state('');

  onMount(() => {
    const node = host;
    if (!node || typeof IntersectionObserver === 'undefined') {
      visible = true;
      return;
    }
    const observer = new IntersectionObserver((entries) => {
      visible = entries.some((entry) => entry.isIntersecting);
    }, { rootMargin: '160px 0px' });
    observer.observe(node);
    return () => observer.disconnect();
  });

  $effect(() => {
    const source = file;
    if (!visible || !source || (!source.type.startsWith('image/') && !source.type.startsWith('video/'))) {
      previewURL = '';
      return;
    }
    const url = URL.createObjectURL(source);
    previewURL = url;
    return () => {
      URL.revokeObjectURL(url);
      if (previewURL === url) previewURL = '';
    };
  });

  function itemIcon() {
    if (item.type?.startsWith('video/')) return 'video';
    if (item.type?.startsWith('audio/')) return 'audio';
    if (item.type === 'image/gif') return 'gif';
    if (item.name.match(/\.(zip|tar|gz)$/i)) return 'folder';
    return 'photo';
  }
</script>

<div bind:this={host} class="thumb-tile" data-upload-preview>
  {#if previewURL && item.type?.startsWith('video/')}
    <!-- svelte-ignore a11y_media_has_caption -->
    <video src={previewURL} muted playsinline preload="metadata"></video>
  {:else if previewURL && item.type?.startsWith('image/')}
    <img src={previewURL} alt="" loading="lazy" decoding="async" />
  {:else}
    <Icon name={itemIcon()} size={18} />
  {/if}
</div>
