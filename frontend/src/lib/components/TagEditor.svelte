<script lang="ts">
  import Icon from './Icon.svelte';
  import TagAutocompleteInput from './TagAutocompleteInput.svelte';
  import type { TagCandidate } from '$lib/utils/tagSuggestions';

  let {
    fileID,
    fileName,
    draft,
    busy,
    error,
    tags,
    existingTags,
    mode = 'add',
    onInput,
    onCommit
  } = $props<{
    fileID: string;
    fileName: string;
    draft: string;
    busy: boolean;
    error: string;
    tags: TagCandidate[];
    existingTags: string[];
    mode?: 'add' | 'remove';
    onInput: (value: string) => void;
    onCommit: (value: string) => void;
  }>();

  const candidates = $derived(mode === 'remove' ? existingTags.map((name: string) => ({ name })) : tags);
  const excluded = $derived(mode === 'remove' ? [] : existingTags);
</script>

<div class="tag-editor">
  <div class="lightbox-tag-input" class:untag-mode={mode === 'remove'}>
    {#if mode === 'remove'}
      <span class="tag-mode-sign" aria-hidden="true">−</span>
    {:else}
      <Icon name="plus" size={12} />
    {/if}
    <TagAutocompleteInput
      id={`tags-${fileID}`}
      value={draft}
      tags={candidates}
      existing={excluded}
      placeholder=""
      readOnly={busy}
      commitOnBlur={false}
      ariaLabel={`${mode === 'remove' ? 'Remove tags from' : 'Tags for'} ${fileName}`}
      {onInput}
      {onCommit}
    />
    {#if !draft}
      <span class="tag-mode-hint" aria-hidden="true">add <u>t</u>ag · <u>u</u>ntag</span>
    {/if}
  </div>
  {#if error}<p class="form-error">{error}</p>{/if}
</div>

<style>
  .tag-editor .lightbox-tag-input {
    position: relative;
  }

  .tag-mode-sign {
    width: 12px;
    flex: 0 0 12px;
    color: var(--text-3);
    font: 16px/12px var(--font-mono);
    text-align: center;
  }

  .untag-mode .tag-mode-sign {
    color: var(--danger);
  }

  .tag-mode-hint {
    position: absolute;
    left: 27px;
    top: 50%;
    transform: translateY(-50%);
    color: var(--text-4);
    font: inherit;
    pointer-events: none;
    white-space: nowrap;
  }

  .tag-mode-hint u {
    text-underline-offset: 2px;
  }
</style>
