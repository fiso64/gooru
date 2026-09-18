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
    onCommit,
    onModeToggle
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
    onModeToggle?: () => void;
  }>();

  const candidates = $derived(mode === 'remove' ? existingTags.map((name: string) => ({ name })) : tags);
  const excluded = $derived(mode === 'remove' ? [] : existingTags);

  function handleModeShortcut(event: KeyboardEvent) {
    if (!onModeToggle || busy || draft || event.isComposing || event.ctrlKey || event.metaKey || event.altKey) return;
    if (event.key !== '+' && event.key !== '-') return;
    event.preventDefault();
    event.stopPropagation();
    const nextMode = event.key === '-' ? 'remove' : 'add';
    if (nextMode !== mode) onModeToggle();
  }
</script>

<div class="tag-editor" onkeydown={handleModeShortcut}>
  <div class="lightbox-tag-input" class:untag-mode={mode === 'remove'}>
    {#if onModeToggle}
      <button
        class="tag-mode-toggle"
        type="button"
        aria-label={mode === 'remove' ? `Switch to adding tags for ${fileName}` : `Switch to removing tags from ${fileName}`}
        title={mode === 'remove' ? 'Switch to tag mode' : 'Switch to untag mode'}
        disabled={busy}
        onclick={onModeToggle}
      >
        {#if mode === 'remove'}
          <span class="tag-mode-sign" aria-hidden="true">−</span>
        {:else}
          <Icon name="plus" size={12} />
        {/if}
      </button>
    {:else if mode === 'remove'}
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

  .tag-editor :global(.tag-autocomplete > input) {
    padding: 0;
    border: 0;
    border-radius: 0;
    background: transparent;
    font: inherit;
  }

  .tag-mode-toggle {
    width: 12px;
    height: 18px;
    flex: 0 0 12px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    border: 0;
    background: transparent;
    color: var(--text-3);
    cursor: pointer;
  }

  .tag-mode-toggle:focus-visible {
    outline: 2px solid var(--accent-line);
    outline-offset: 2px;
    border-radius: 2px;
  }

  .tag-mode-toggle:disabled {
    cursor: default;
    opacity: 0.55;
  }

  .tag-mode-sign {
    width: 12px;
    flex: 0 0 12px;
    color: var(--text-3);
    font: 16px/12px var(--font-mono);
    text-align: center;
  }

  .untag-mode .tag-mode-toggle,
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
