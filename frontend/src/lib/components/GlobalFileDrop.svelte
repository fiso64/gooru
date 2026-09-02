<script lang="ts">
  import Icon from './Icon.svelte';
  import { hasDraggedFiles, pastedMediaFiles } from '$lib/utils/fileDrop';

  let { onFiles } = $props<{ onFiles: (files: FileList | File[]) => void }>();

  let dragDepth = $state(0);
  const active = $derived(dragDepth > 0);

  function isFileDrag(event: DragEvent) {
    return hasDraggedFiles(event.dataTransfer?.types);
  }

  function handleDragEnter(event: DragEvent) {
    if (!isFileDrag(event)) return;
    event.preventDefault();
    dragDepth += 1;
  }

  function handleDragOver(event: DragEvent) {
    if (!isFileDrag(event)) return;
    event.preventDefault();
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy';
    if (dragDepth === 0) dragDepth = 1;
  }

  function handleDragLeave(event: DragEvent) {
    if (!active) return;
    if (event.relatedTarget == null) {
      dragDepth = 0;
      return;
    }
    dragDepth = Math.max(0, dragDepth - 1);
  }

  function handleDrop(event: DragEvent) {
    if (!isFileDrag(event)) return;
    dragDepth = 0;

    // The upload panel retains its local drop zone. Let that handler own drops
    // when it already consumed the event so files are never staged twice.
    if (event.defaultPrevented) return;

    event.preventDefault();
    const files = event.dataTransfer?.files;
    if (files?.length) onFiles(files);
  }

  function handlePaste(event: ClipboardEvent) {
    const files = pastedMediaFiles(event.clipboardData?.files);
    if (!files.length) return;

    // Only consume the paste when the clipboard actually contains media files.
    // Plain text and other clipboard data continue to the focused control.
    event.preventDefault();
    onFiles(files);
  }
</script>

<svelte:window
  ondragenter={handleDragEnter}
  ondragover={handleDragOver}
  ondragleave={handleDragLeave}
  ondrop={handleDrop}
  onpaste={handlePaste}
/>

{#if active}
  <div class="global-file-drop" role="status" aria-live="polite">
    <div class="global-file-drop-card">
      <span class="global-file-drop-icon"><Icon name="upload" size={30} /></span>
      <strong>Drop files to upload</strong>
      <span>Release anywhere in Gooru</span>
    </div>
  </div>
{/if}

<style>
  .global-file-drop {
    position: fixed;
    z-index: 1000;
    inset: 0;
    display: grid;
    place-items: center;
    padding: 24px;
    pointer-events: none;
    background: color-mix(in srgb, var(--bg) 72%, transparent);
    backdrop-filter: blur(4px);
  }

  .global-file-drop::before {
    content: '';
    position: absolute;
    inset: 16px;
    border: 2px dashed var(--accent);
    border-radius: var(--r-4);
    background: color-mix(in srgb, var(--accent) 7%, transparent);
  }

  .global-file-drop-card {
    position: relative;
    display: grid;
    justify-items: center;
    gap: 10px;
    min-width: min(420px, calc(100vw - 64px));
    padding: 34px 42px;
    border: 1px solid var(--accent-line);
    border-radius: var(--r-4);
    background: var(--bg-2);
    box-shadow: 0 24px 80px rgba(0, 0, 0, 0.45);
    color: var(--text);
  }

  .global-file-drop-card strong {
    font-size: 18px;
  }

  .global-file-drop-card > span:last-child {
    color: var(--text-3);
    font-size: 13px;
  }

  .global-file-drop-icon {
    display: grid;
    place-items: center;
    width: 58px;
    height: 58px;
    border-radius: 50%;
    background: var(--accent-soft);
    color: var(--accent);
  }
</style>
