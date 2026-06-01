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
      placeholder="add tag - e.g. subject:portrait"
      oninput={(event) => onInput(event.currentTarget.value)}
      onkeydown={(event) => { if (event.key === 'Enter' && canSubmit && !busy) onMutate('add'); }}
    />
  </div>
  <div class="tag-editor-actions">
    <button class="g-btn g-btn-sm" type="button" aria-label={`Add tags to ${fileName}`} disabled={!canSubmit || busy} onclick={() => onMutate('add')}>
      <Icon name="plus" size={12} /> Add
    </button>
    <button class="g-btn g-btn-sm" type="button" aria-label={`Set tags on ${fileName}`} disabled={!canSubmit || busy} onclick={() => onMutate('set')}>
      <Icon name="check" size={12} /> Set
    </button>
    <button class="g-btn g-btn-sm" type="button" aria-label={`Remove tags from ${fileName}`} disabled={!canSubmit || busy} onclick={() => onMutate('remove')}>
      <Icon name="trash" size={12} /> Remove
    </button>
  </div>
  {#if error}<p class="form-error">{error}</p>{/if}
</div>
