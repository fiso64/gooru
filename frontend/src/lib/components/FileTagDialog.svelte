<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import TagEditor from './TagEditor.svelte';
  import { groupTags, parseTags } from '$lib/utils/format';
  import type { FileItem } from '$lib/api/types';
  import type { TagCandidate } from '$lib/utils/tagSuggestions';

  let {
    file,
    tagCandidates,
    initialMode = 'add',
    busy = false,
    error = '',
    onCancel,
    onConfirm
  } = $props<{
    file: FileItem;
    tagCandidates: TagCandidate[];
    initialMode?: 'add' | 'remove';
    busy?: boolean;
    error?: string;
    onCancel: () => void;
    onConfirm: (tags: string[]) => void;
  }>();

  let dialogRef = $state<HTMLDivElement | undefined>();
  let mode = $state<'add' | 'remove'>(initialMode);
  let draft = $state('');
  let stagedTags = $state([...file.tags]);
  const tagGroups = $derived(groupTags(stagedTags));
  const hasTagNamespaces = $derived(tagGroups.some((group) => Boolean(group.namespace)));

  function applyDraft(value: string, tags = stagedTags) {
    const parsed = parseTags(value);
    if (!parsed.length) return tags;
    if (mode === 'add') return Array.from(new Set([...tags, ...parsed]));
    const removed = new Set(parsed);
    return tags.filter((tag) => !removed.has(tag));
  }

  function commitDraft(value: string) {
    stagedTags = applyDraft(value);
    draft = '';
  }

  function removeTag(tag: string) {
    stagedTags = stagedTags.filter((candidate) => candidate !== tag);
  }

  function toggleMode() {
    draft = '';
    mode = mode === 'add' ? 'remove' : 'add';
  }

  function confirm() {
    const nextTags = draft.trim() ? applyDraft(draft) : stagedTags;
    stagedTags = nextTags;
    draft = '';
    onConfirm(nextTags);
  }

  onMount(() => {
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    queueMicrotask(() => dialogRef?.querySelector<HTMLInputElement>('input:not([disabled])')?.focus());

    function handleKeydown(event: KeyboardEvent) {
      if (!dialogRef) return;
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopImmediatePropagation();
        if (!busy) onCancel();
        return;
      }
      if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
        event.preventDefault();
        event.stopImmediatePropagation();
        if (!busy) confirm();
        return;
      }
      if (event.key !== 'Tab') return;

      const focusable = Array.from(
        dialogRef.querySelectorAll<HTMLElement>('input:not([disabled]), button:not([disabled]), [href], [tabindex]:not([tabindex="-1"])')
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

<div class="modal-backdrop" role="presentation">
  <div bind:this={dialogRef} class="action-dialog file-tag-dialog" role="dialog" aria-modal="true" aria-labelledby="file-tag-dialog-title" tabindex="-1">
    <div class="file-tag-heading">
      <div>
        <div class="g-eyebrow g-eyebrow-accent">Tags</div>
        <h2 id="file-tag-dialog-title">Edit tags · {file.name}</h2>
      </div>
      <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" aria-label="Close tag editor" disabled={busy} onclick={onCancel}>
        <Icon name="close" size={14} />
      </button>
    </div>

    <div class="file-tag-list" aria-label={"Tags for " + file.name}>
      {#if !stagedTags.length}
        <span class="file-tag-empty">No tags</span>
      {:else}
        {#each tagGroups as group (group.namespace)}
          <div class="file-tag-group">
            {#if group.namespace || hasTagNamespaces}
              <div class="lightbox-tag-group-head"><span>{group.namespace || 'OTHER'}</span><span>{group.tags.length}</span></div>
            {/if}
            <div class="lightbox-tag-list">
              {#each group.tags as tag}
                <span class="g-tag">
                  {#if tag.includes(':')}
                    <span class="ns">{tag.split(':')[0]}:</span><span>{tag.slice(tag.indexOf(':') + 1)}</span>
                  {:else}
                    <span>{tag}</span>
                  {/if}
                  <button class="g-tag-x" type="button" aria-label={"Stage removal of " + tag} disabled={busy} onclick={() => removeTag(tag)}>
                    <Icon name="close" size={11} />
                  </button>
                </span>
              {/each}
            </div>
          </div>
        {/each}
      {/if}
    </div>

    <TagEditor
      fileID={file.id}
      fileName={file.name}
      {draft}
      {busy}
      {error}
      tags={tagCandidates}
      existingTags={stagedTags}
      {mode}
      onInput={(value) => (draft = value)}
      onCommit={commitDraft}
      onModeToggle={toggleMode}
    />

    <div class="dialog-actions">
      <button class="g-btn g-btn-sm" type="button" disabled={busy} onclick={onCancel}>Cancel</button>
      <button class="g-btn g-btn-sm" type="button" disabled={busy} onclick={confirm}>{busy ? 'Working' : 'Apply'}</button>
    </div>
  </div>
</div>

<style>
  .file-tag-dialog {
    width: min(620px, calc(100vw - 40px));
  }

  .file-tag-heading {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 18px;
  }

  .file-tag-heading h2 {
    margin-top: 5px;
  }

  .file-tag-list {
    max-height: min(320px, 42vh);
    overflow: auto;
    display: grid;
    gap: 10px;
    padding: 10px 0 2px;
  }

  .file-tag-group {
    display: grid;
    gap: 6px;
  }

  .file-tag-empty {
    color: var(--text-3);
    font-size: 12px;
  }
</style>
