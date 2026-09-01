<script lang="ts">
  import { isPlainTag, plainTagSuggestions, type TagCandidate } from '$lib/utils/tagSuggestions';

  let {
    value,
    tags,
    existing = [],
    placeholder = 'add tag',
    disabled = false,
    ariaLabel = 'Tag',
    onInput,
    onCommit
  } = $props<{
    value: string;
    tags: TagCandidate[];
    existing?: string[];
    placeholder?: string;
    disabled?: boolean;
    ariaLabel?: string;
    onInput: (value: string) => void;
    onCommit: (value: string) => void;
  }>();

  let open = $state(false);
  let active = $state(0);
  let inputRef = $state<HTMLInputElement | undefined>();
  const suggestions = $derived(plainTagSuggestions(value, tags, existing));

  $effect(() => {
    if (active >= suggestions.length) active = 0;
    if (!value.trim()) open = false;
  });

  function commit(raw: string) {
    const tag = raw.trim();
    if (!isPlainTag(tag) || existing.includes(tag)) return;
    open = false;
    active = 0;
    onCommit(tag);
  }

  function handleInput(event: Event) {
    const next = (event.currentTarget as HTMLInputElement).value;
    onInput(next);
    open = Boolean(next.trim());
    active = 0;
  }

  function handleKeydown(event: KeyboardEvent) {
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
      commit(suggestions[active].name);
      return;
    }
    if (event.key === 'Enter' && value.trim()) {
      event.preventDefault();
      commit(value);
      return;
    }
    if (event.key === 'Escape') {
      event.preventDefault();
      open = false;
      onInput('');
      inputRef?.blur();
    }
  }
</script>

<div class="tag-autocomplete">
  <input
    bind:this={inputRef}
    value={value}
    {placeholder}
    {disabled}
    aria-label={ariaLabel}
    aria-expanded={open && suggestions.length > 0}
    aria-haspopup="listbox"
    autocomplete="off"
    spellcheck="false"
    oninput={handleInput}
    onfocus={() => (open = Boolean(value.trim()))}
    onkeydown={handleKeydown}
  />

  {#if open && suggestions.length > 0}
    <ul class="tag-autocomplete-list" role="listbox" aria-label={`${ariaLabel} suggestions`} onmousedown={(event) => event.preventDefault()}>
      {#each suggestions as suggestion, index}
        <li role="presentation">
          <button
            class:is-active={index === active}
            type="button"
            role="option"
            aria-selected={index === active}
            onmouseenter={() => (active = index)}
            onclick={() => commit(suggestion.name)}
          >
            {@const separator = suggestion.name.indexOf(':')}
            <span>
              {#if separator > 0}<span class="ns">{suggestion.name.slice(0, separator)}:</span>{suggestion.name.slice(separator + 1)}{:else}{suggestion.name}{/if}
            </span>
            {#if suggestion.count}<span class="count">{suggestion.count.toLocaleString()}</span>{/if}
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
    z-index: 90;
    top: calc(100% + 6px);
    left: 0;
    width: min(360px, 80vw);
    max-height: 260px;
    overflow: auto;
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

  .tag-autocomplete-list .ns {
    color: var(--accent);
  }

  .tag-autocomplete-list .count {
    color: var(--text-4);
    font-family: var(--font-mono);
    font-size: 10.5px;
  }
</style>
