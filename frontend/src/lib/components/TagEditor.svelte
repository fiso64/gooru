<script lang="ts">
  import Icon from './Icon.svelte';

  let {
    fileID,
    fileName,
    draft,
    busy,
    error,
    canSubmit,
    onInput,
    onMutate
  } = $props<{
    fileID: string;
    fileName: string;
    draft: string;
    busy: boolean;
    error: string;
    canSubmit: boolean;
    onInput: (value: string) => void;
    onMutate: (operation: 'add' | 'set' | 'remove') => void;
  }>();
</script>

<div class="tag-editor">
  <label class="sr-only" for={`tags-${fileID}`}>Tags for {fileName}</label>
  <div class="lightbox-tag-input">
    <Icon name="plus" size={12} />
    <input
      id={`tags-${fileID}`}
      value={draft}
      placeholder="add tag — e.g. subject:portrait"
      disabled={busy}
      oninput={(event) => onInput(event.currentTarget.value)}
      onkeydown={(event) => { if (event.key === 'Enter' && canSubmit && !busy) onMutate('add'); }}
    />
  </div>
  {#if error}<p class="form-error">{error}</p>{/if}
</div>
