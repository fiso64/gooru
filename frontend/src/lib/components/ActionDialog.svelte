<script lang="ts">
  import { flushSync, onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import TagAutocompleteInput from './TagAutocompleteInput.svelte';
  import { parseTags } from '$lib/utils/format';
  import { placeModalInFullscreenViewer } from '$lib/utils/modal';
  import type { TagCandidate } from '$lib/utils/tagSuggestions';

  let {
    title,
    description,
    label,
    value,
    confirmText = 'Apply',
    destructive = false,
    busy = false,
    error = '',
    input = true,
    tagInput = false,
    tagCandidates = [],
    onInput,
    onCancel,
    onConfirm
  } = $props<{
    title: string;
    description: string;
    label?: string;
    value?: string;
    confirmText?: string;
    destructive?: boolean;
    busy?: boolean;
    error?: string;
    input?: boolean;
    tagInput?: boolean;
    tagCandidates?: TagCandidate[];
    onInput?: (value: string) => void;
    onCancel: () => void;
    onConfirm: () => void;
  }>();

  let dialogRef: HTMLDivElement | undefined;
  let tagDraft = $state('');
  let committedTags = $state<string[]>([]);

  function syncTagValue(tags: string[]) {
    committedTags = Array.from(new Set(tags));
    onInput?.(committedTags.join(' '));
  }

  function commitTagInput(raw: string) {
    syncTagValue([...committedTags, ...parseTags(raw)]);
    tagDraft = '';
  }

  function removeTag(tag: string) {
    syncTagValue(committedTags.filter((candidate) => candidate !== tag));
  }

  function isEditableTarget(target: EventTarget | null) {
    if (!(target instanceof Element)) return false;
    return Boolean(target.closest('input, textarea, [contenteditable=""], [contenteditable="true"]'));
  }

  function hasOpenTagCompletions(target: EventTarget | null) {
    return target instanceof HTMLInputElement
      && target.getAttribute('aria-haspopup') === 'listbox'
      && target.getAttribute('aria-expanded') === 'true';
  }

  onMount(() => {
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    if (tagInput) committedTags = parseTags(value ?? '');

    queueMicrotask(() => {
      if (!input) {
        dialogRef?.focus();
        return;
      }
      const first = dialogRef?.querySelector<HTMLElement>('input:not([disabled]), textarea:not([disabled]), [contenteditable="true"]');
      (first ?? dialogRef)?.focus();
    });

    function handleKeydown(event: KeyboardEvent) {
      if (!dialogRef) return;
      if (event.key === 'Escape') {
        if (hasOpenTagCompletions(event.target)) return;
        event.preventDefault();
        event.stopImmediatePropagation();
        if (!busy) onCancel();
        return;
      }
      if (event.key === 'Enter') {
        const modifiedSubmit = event.ctrlKey || event.metaKey;
        if (!modifiedSubmit && isEditableTarget(event.target)) return;
        event.preventDefault();
        event.stopImmediatePropagation();
        if (!busy) {
          if (modifiedSubmit && tagInput && tagDraft.trim()) {
            commitTagInput(tagDraft);
            flushSync();
          }
          onConfirm();
        }
        return;
      }
      if (event.key !== 'Tab') return;

      const focusable = Array.from(
        dialogRef.querySelectorAll<HTMLElement>('input:not([disabled]), textarea:not([disabled]), button:not([disabled]), [href], [tabindex]:not([tabindex="-1"])')
      ).filter((element) => !element.hasAttribute('hidden'));
      if (!focusable.length) {
        event.preventDefault();
        dialogRef.focus();
        return;
      }

      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const current = document.activeElement;
      if (event.shiftKey && (current === first || !dialogRef.contains(current))) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && (current === last || !dialogRef.contains(current))) {
        event.preventDefault();
        first.focus();
      }
    }

    window.addEventListener('keydown', handleKeydown, true);
    return () => {
      window.removeEventListener('keydown', handleKeydown, true);
      if (previousFocus?.isConnected) previousFocus.focus();
    };
  });
</script>

<div use:placeModalInFullscreenViewer class="modal-backdrop" role="presentation">
  <div bind:this={dialogRef} class="action-dialog" role="dialog" aria-modal="true" aria-labelledby="action-dialog-title" tabindex="-1">
    <h2 id="action-dialog-title">{title}</h2>
    <p>{description}</p>
    {#if input}
      <label>
        <span>{label}</span>
        {#if tagInput}
          <div class="dialog-tag-input">
            {#each committedTags as tag}
              <span class="g-tag">
                {#if tag.includes(':')}
                  <span class="g-tag-ns">{tag.slice(0, tag.indexOf(':'))}:</span><span>{tag.slice(tag.indexOf(':') + 1)}</span>
                {:else}
                  <span>{tag}</span>
                {/if}
                <button class="g-tag-x" type="button" disabled={busy} aria-label={`Remove ${tag}`} onclick={() => removeTag(tag)}>
                  <Icon name="close" size={11} />
                </button>
              </span>
            {/each}
            <TagAutocompleteInput
              value={tagDraft}
              tags={tagCandidates}
              existing={committedTags}
              placeholder="add tag"
              readOnly={busy}
              ariaLabel={label ?? 'Tags'}
              onInput={(next) => (tagDraft = next)}
              onCommit={commitTagInput}
              onRemoveLast={removeTag}
            />
          </div>
        {:else}
          <input
            value={value ?? ''}
            disabled={busy}
            oninput={(event) => onInput?.(event.currentTarget.value)}
          />
        {/if}
      </label>
    {/if}
    {#if error}<div class="dialog-error">{error}</div>{/if}
    <div class="dialog-actions">
      <button class="g-btn g-btn-sm" type="button" disabled={busy} onclick={onCancel}>Cancel</button>
      <button class:danger={destructive} class="g-btn g-btn-sm" type="button" disabled={busy} onclick={onConfirm}>{busy ? 'Working' : confirmText}</button>
    </div>
  </div>
</div>

<style>
  .dialog-tag-input {
    min-height: 38px;
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 5px;
    padding: 6px 9px;
    border: 1px solid var(--border);
    border-radius: var(--r-3);
    background: var(--surface);
  }

  .dialog-tag-input:focus-within {
    border-color: var(--accent-line);
    background: var(--bg-2);
    box-shadow: 0 0 0 3px var(--accent-soft);
  }

  .dialog-tag-input :global(.tag-autocomplete-list) {
    top: auto;
    bottom: calc(100% + 6px);
  }
</style>
