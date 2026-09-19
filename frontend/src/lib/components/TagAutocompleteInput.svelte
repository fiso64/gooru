<script lang="ts">
  import { tick } from 'svelte';
  import { createSuggestionsQuery } from '$lib/queries/library';
  import { authState } from '$lib/stores/auth';
  import { mergeTagCandidateOccurrenceCounts, plainTagSuggestions, plainTagsFromInput, type PlainTagSuggestion, type TagCandidate } from '$lib/utils/tagSuggestions';
  import { keepActiveCompletionVisible } from '$lib/utils/completionVisibility';
  import { isEmptyViewerTagShortcut } from '$lib/utils/viewerTagKeyRouting';

  let {
    id,
    value,
    tags,
    existing = [],
    localOnly = false,
    viewerFileID,
    stagedCandidates = [],
    placeholder = 'add tag',
    disabled = false,
    readOnly = false,
    commitOnBlur = true,
    ariaLabel = 'Tag',
    onInput,
    onCommit,
    onRemoveLast,
    onKeydown
  } = $props<{
    id?: string;
    value: string;
    tags: TagCandidate[];
    existing?: string[];
    localOnly?: boolean;
    viewerFileID?: string;
    stagedCandidates?: TagCandidate[];
    placeholder?: string;
    disabled?: boolean;
    readOnly?: boolean;
    commitOnBlur?: boolean;
    ariaLabel?: string;
    onInput: (value: string) => void;
    onCommit: (value: string) => void;
    onRemoveLast?: (tag: string) => void;
    onKeydown?: (event: KeyboardEvent) => void;
  }>();

  let open = $state(false);
  let active = $state(0);
  let inputRef = $state<HTMLInputElement | undefined>();
  let suggestionsRef = $state<HTMLUListElement | undefined>();
  let suppressBlurCommit = false;
  const indexedSuggestions = createSuggestionsQuery(
    () => Boolean($authState.user) && !localOnly && !readOnly,
    () => value,
    () => existing.join(' '),
    () => $authState.user?.username ?? ''
  );
  const completionCandidates = $derived(localOnly || !$authState.user
    ? tags
    : mergeTagCandidateOccurrenceCounts(indexedSuggestions.data?.items ?? [], stagedCandidates));
  const suggestions = $derived(readOnly ? [] : plainTagSuggestions(value, completionCandidates, existing));

  $effect(() => {
    if (active >= suggestions.length) active = 0;
    if (!value.trim() || readOnly) open = false;
  });

  $effect(() => {
    active;
    if (!open) return;
    void tick().then(() => keepActiveCompletionVisible(suggestionsRef));
  });

  function commit(raw: string) {
    if (readOnly) return;
    const nextTags = plainTagsFromInput(raw).filter((tag) => !existing.includes(tag));
    open = false;
    active = 0;
    if (!nextTags.length) return;
    onCommit(nextTags.join(' '));
  }

  function selectSuggestion(suggestion: PlainTagSuggestion) {
    if (readOnly) return;
    if (suggestion.kind === 'namespace') {
      onInput(suggestion.name);
      open = true;
      active = 0;
      inputRef?.focus();
      return;
    }
    commit(suggestion.name);
  }

  function handleInput(event: Event) {
    if (readOnly) return;
    const next = (event.currentTarget as HTMLInputElement).value;
    onInput(next);
    open = Boolean(next.trim());
    active = 0;
  }

  function handleBlur() {
    open = false;
    if (readOnly || !commitOnBlur) return;
    if (suppressBlurCommit) {
      suppressBlurCommit = false;
      return;
    }
    if (value.trim()) commit(value);
  }

  function handleKeydown(event: KeyboardEvent) {
    if (readOnly || event.isComposing) return;
    if (viewerFileID && isEmptyViewerTagShortcut(event, viewerFileID)) {
      // Escape belongs to this input even when other non-text keys route to the viewer.
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        open = false;
        suppressBlurCommit = true;
        inputRef?.blur();
      }
      return;
    }
    onKeydown?.(event);
    if (event.defaultPrevented) return;
    if (event.key === 'ArrowDown' && suggestions.length) {
      event.preventDefault();
      open = true;
      active = Math.min(active + 1, suggestions.length - 1);
      return;
    }
    if (event.key === 'ArrowUp' && suggestions.length) {
      event.preventDefault();
      open = true;
      active = Math.max(active - 1, 0);
      return;
    }
    if ((event.key === 'Enter' || event.key === 'Tab') && open && suggestions[active]) {
      event.preventDefault();
      selectSuggestion(suggestions[active]);
      return;
    }
    if ((event.key === 'Enter' || event.key === ' ') && value.trim()) {
      event.preventDefault();
      commit(value);
      return;
    }
    if (event.key === 'Backspace' && !value && existing.length && onRemoveLast) {
      event.preventDefault();
      onRemoveLast(existing[existing.length - 1]);
      return;
    }
    if (event.key === 'Escape') {
      event.preventDefault();
      if (open && suggestions.length > 0) {
        event.stopPropagation();
        open = false;
        active = 0;
        return;
      }
      open = false;
      suppressBlurCommit = true;
      onInput('');
      inputRef?.blur();
    }
  }
</script>

<div class="tag-autocomplete">
  <input
    {id}
    bind:this={inputRef}
    value={value}
    {placeholder}
    {disabled}
    readonly={readOnly}
    aria-label={ariaLabel}
    aria-expanded={open && suggestions.length > 0}
    aria-haspopup="listbox"
    aria-busy={readOnly}
    autocomplete="off"
    spellcheck="false"
    oninput={handleInput}
    onfocus={() => (open = !readOnly && Boolean(value.trim()))}
    onblur={handleBlur}
    onkeydown={handleKeydown}
  />

  {#if open && suggestions.length > 0}
    <ul bind:this={suggestionsRef} class="tag-autocomplete-list" role="listbox" aria-label={`${ariaLabel} suggestions`} onmousedown={(event) => event.preventDefault()}>
      {#each suggestions as suggestion, index}
        <li role="presentation">
          <button
            class:is-active={index === active}
            type="button"
            role="option"
            aria-selected={index === active}
            onmouseenter={() => (active = index)}
            onclick={() => selectSuggestion(suggestion)}
          >
            <span class="name" title={suggestion.name}>
              {#if suggestion.name.includes(':')}
                <span class="ns">{suggestion.name.slice(0, suggestion.name.indexOf(':'))}:</span>{suggestion.name.slice(suggestion.name.indexOf(':') + 1)}
              {:else}
                {suggestion.name}
              {/if}
            </span>
            {#if suggestion.kind === 'namespace'}
              <span class="count">namespace</span>
            {:else if suggestion.count}
              <span class="count">{suggestion.count.toLocaleString()}</span>
            {/if}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .tag-autocomplete {
    position: relative;
    flex: 1 1 180px;
    min-width: 0;
  }

  .tag-autocomplete > input {
    width: 100%;
    min-width: 0;
    border: 0;
    outline: 0;
    background: transparent;
    color: var(--text);
    font: inherit;
  }

  .tag-autocomplete > input::placeholder {
    color: var(--text-4);
  }

  .tag-autocomplete-list {
    position: absolute;
    z-index: 1000;
    top: calc(100% + 6px);
    left: 0;
    width: min(360px, 100%);
    max-height: 260px;
    box-sizing: border-box;
    overflow-y: auto;
    overflow-x: hidden;
    margin: 0;
    padding: 4px;
    list-style: none;
    border: 1px solid var(--border-strong);
    border-radius: var(--r-3);
    background: var(--bg-2);
    box-shadow: 0 12px 36px rgba(0, 0, 0, 0.35);
  }

  .tag-autocomplete-list button {
    width: 100%;
    min-width: 0;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 7px 9px;
    border: 0;
    border-radius: var(--r-2);
    background: transparent;
    color: var(--text-2);
    text-align: left;
    cursor: pointer;
  }

  .tag-autocomplete-list button:hover,
  .tag-autocomplete-list button.is-active {
    background: var(--surface-2);
    color: var(--text);
  }

  .tag-autocomplete-list .name {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .tag-autocomplete-list .ns {
    color: var(--accent);
  }

  .tag-autocomplete-list .count {
    flex: 0 0 auto;
    color: var(--text-4);
    font-family: var(--font-mono);
    font-size: 10.5px;
  }
</style>
