<script lang="ts">
  import Icon from './Icon.svelte';
  import { parseSearchQuery, parseSearchToken, searchTokensToQuery, searchTokenToString, type SearchToken } from '$lib/search/tokens';
  import { isEditableShortcutTarget } from '$lib/utils/keyboard';

  type TagLike = { name?: string; tag?: string; namespace?: string; value?: string; count?: number };
  type SuggestionLike = { name: string; count?: number };
  type SuggestionItem = {
    kind: 'namespace' | 'tag' | 'valueless' | 'value';
    commit: string;
    ns: string;
    val: string;
    count?: number;
    hint?: string;
    partial?: boolean;
  };
  type SuggestionGroup = { head: string; items: SuggestionItem[] };

  let {
    value,
    suggestions,
    tags,
    onDraftInput,
    onCommit
  } = $props<{
    value: string;
    suggestions: SuggestionLike[];
    tags: TagLike[];
    onDraftInput: (value: string) => void;
    onCommit: (value: string) => void;
  }>();

  let tokens = $state<SearchToken[]>([]);
  let draft = $state('');
  let open = $state(false);
  let active = $state(0);
  let inputRef = $state<HTMLInputElement | undefined>();
  let rootRef = $state<HTMLDivElement | undefined>();
  let lastSyncedValue = $state('');

  const tagItems = $derived(normalizeTags(tags, suggestions));
  const namespaceItems = $derived(buildNamespaceCounts(tagItems));
  const groups = $derived(computeSuggestions(draft, tokens, tagItems, namespaceItems));
  const flat = $derived(groups.flatMap((group) => group.items.map((item) => ({ ...item, group: group.head }))));

  $effect(() => {
    if (value === lastSyncedValue) return;
    tokens = parseSearchQuery(value);
    draft = '';
    open = false;
    lastSyncedValue = value;
  });

  $effect(() => {
    if (active >= flat.length) active = 0;
  });

  $effect(() => {
    const handleKeydown = (event: KeyboardEvent) => {
      if (event.key === '/' && !isEditableShortcutTarget(event.target)) {
        event.preventDefault();
        inputRef?.focus();
      }
    };
    window.addEventListener('keydown', handleKeydown);
    return () => window.removeEventListener('keydown', handleKeydown);
  });

  $effect(() => {
    if (!open) return;
    const handleMouseDown = (event: MouseEvent) => {
      if (rootRef && !rootRef.contains(event.target as Node)) open = false;
    };
    document.addEventListener('mousedown', handleMouseDown);
    return () => document.removeEventListener('mousedown', handleMouseDown);
  });

  function normalizeTags(tags: TagLike[], suggestions: SuggestionLike[]) {
    const seen = new Map<string, { tag: string; count: number }>();
    for (const tag of tags) {
      const name = tag.name ?? tag.tag ?? (tag.namespace ? `${tag.namespace}:${tag.value ?? ''}` : (tag.value ?? ''));
      if (!name) continue;
      seen.set(name, { tag: name, count: tag.count ?? seen.get(name)?.count ?? 0 });
    }
    for (const suggestion of suggestions) {
      if (!suggestion.name) continue;
      seen.set(suggestion.name, { tag: suggestion.name, count: suggestion.count ?? seen.get(suggestion.name)?.count ?? 0 });
    }
    return [...seen.values()].sort((a, b) => b.count - a.count || a.tag.localeCompare(b.tag));
  }

  function parseTag(tag: string) {
    const colon = tag.indexOf(':');
    if (colon < 0) return { ns: '', value: tag };
    return { ns: tag.slice(0, colon), value: tag.slice(colon + 1) };
  }

  function buildNamespaceCounts(items: Array<{ tag: string; count: number }>) {
    const counts = new Map<string, number>();
    for (const item of items) {
      const parsed = parseTag(item.tag);
      if (!parsed.ns || item.tag.startsWith('@')) continue;
      counts.set(parsed.ns, (counts.get(parsed.ns) ?? 0) + item.count);
    }
    return [...counts.entries()]
      .map(([ns, count]) => ({ ns, count }))
      .sort((a, b) => b.count - a.count || a.ns.localeCompare(b.ns));
  }

  function computeSuggestions(
    draftValue: string,
    currentTokens: SearchToken[],
    items: Array<{ tag: string; count: number }>,
    namespaces: Array<{ ns: string; count: number }>
  ): SuggestionGroup[] {
    const tokenStrings = new Set(currentTokens.map(searchTokenToString));
    const takenPositiveTags = new Set(currentTokens.filter((token) => !token.neg).map((token) => `${token.ns}:${token.val}`));
    let working = draftValue.trim();
    let negPrefix = '';
    if (working.startsWith('-')) {
      negPrefix = '-';
      working = working.slice(1);
    }

    if (!working.includes(':')) {
      const term = working.toLowerCase();
      const result: SuggestionGroup[] = [];
      const nsItems = namespaces
        .filter(({ ns }) => !term || ns.toLowerCase().startsWith(term))
        .slice(0, 6)
        .map(({ ns, count }) => ({ kind: 'namespace' as const, commit: `${negPrefix}${ns}:`, ns, val: '', count, hint: 'namespace', partial: true }));
      if (nsItems.length) result.push({ head: 'Namespaces', items: nsItems });

      const matchingTags = items
        .filter(({ tag }) => {
          if (takenPositiveTags.has(tag) || tokenStrings.has(`${negPrefix}${tag}`)) return false;
          const parsed = parseTag(tag);
          return term ? parsed.value.toLowerCase().includes(term) : !parsed.ns;
        })
        .slice(0, term ? 8 : 5)
        .map(({ tag, count }) => {
          const parsed = parseTag(tag);
          return {
            kind: parsed.ns ? ('tag' as const) : ('valueless' as const),
            commit: `${negPrefix}${tag}`,
            ns: parsed.ns,
            val: parsed.value,
            count
          };
        });
      if (matchingTags.length) result.push({ head: term ? 'Tags' : 'Valueless', items: matchingTags });
      return result;
    }

    const colon = working.indexOf(':');
    const ns = working.slice(0, colon).toLowerCase();
    const valFrag = working.slice(colon + 1).toLowerCase();
    const values = items
      .filter(({ tag }) => {
        if (tag.startsWith('@') || takenPositiveTags.has(tag)) return false;
        const parsed = parseTag(tag);
        if (parsed.ns.toLowerCase() !== ns) return false;
        return !valFrag || parsed.value.toLowerCase().includes(valFrag);
      })
      .slice(0, 12)
      .map(({ tag, count }) => {
        const parsed = parseTag(tag);
        return { kind: 'value' as const, commit: `${negPrefix}${tag}`, ns: parsed.ns, val: parsed.value, count };
      });
    return values.length ? [{ head: `${ns}:`, items: values }] : [];
  }

  function syncCommit(nextTokens: SearchToken[]) {
    tokens = nextTokens;
    const query = searchTokensToQuery(tokens);
    lastSyncedValue = query;
    onCommit(query);
  }

  function commitString(input: string) {
    const value = input.trim();
    if (!value) return;
    const token = parseSearchToken(value);
    if (!token.ns && !token.val) return;
    const key = searchTokenToString(token);
    if (tokens.some((item) => searchTokenToString(item) === key)) {
      draft = '';
      open = false;
      onDraftInput('');
      return;
    }
    draft = '';
    open = false;
    active = 0;
    onDraftInput('');
    syncCommit([...tokens, token]);
  }

  function selectSuggestion(item: SuggestionItem) {
    if (item.partial) {
      draft = item.commit;
      active = 0;
      open = true;
      onDraftInput(draft);
      inputRef?.focus();
      return;
    }
    commitString(item.commit);
  }

  function removeToken(index: number) {
    syncCommit(tokens.filter((_, tokenIndex) => tokenIndex !== index));
  }

  function clearAll() {
    draft = '';
    open = false;
    active = 0;
    onDraftInput('');
    syncCommit([]);
  }

  function handleInput(event: Event) {
    draft = (event.currentTarget as HTMLInputElement).value;
    open = Boolean(draft.trim());
    active = 0;
    onDraftInput(draft);
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'ArrowDown') {
      if (!draft.trim()) return;
      event.preventDefault();
      active = Math.min(active + 1, Math.max(flat.length - 1, 0));
      open = true;
      return;
    }
    if (event.key === 'ArrowUp') {
      if (!draft.trim()) return;
      event.preventDefault();
      active = Math.max(active - 1, 0);
      open = true;
      return;
    }
    if (event.key === 'Enter' || event.key === 'Tab') {
      if (open && flat[active]) {
        event.preventDefault();
        selectSuggestion(flat[active]);
      } else if (draft.trim()) {
        event.preventDefault();
        commitString(draft);
      }
      return;
    }
    if (event.key === 'Escape') {
      event.preventDefault();
      open = false;
      draft = '';
      onDraftInput('');
      inputRef?.blur();
      return;
    }
    if (event.key === 'Backspace' && !draft && tokens.length > 0) {
      event.preventDefault();
      removeToken(tokens.length - 1);
    }
  }
</script>

<div
  class="searchbar"
  role="combobox"
  aria-label="Search tokens"
  aria-controls="searchbar-suggestions"
  aria-expanded={open && flat.length > 0 ? 'true' : 'false'}
  aria-haspopup="listbox"
  tabindex="-1"
  bind:this={rootRef}
  onclick={() => inputRef?.focus()}
>
  <span class="searchbar-icon"><Icon name="search" size={15} /></span>
  {#each tokens as token, index}
    <span class:neg={token.neg} class="searchbar-pill">
      {#if token.neg}<span class="neg-symbol">−</span>{/if}
      {#if token.ns}<span class="ns">{token.ns}:</span>{/if}<span>{token.val}</span>
      <button class="x" type="button" aria-label={`Remove ${searchTokenToString(token)}`} onclick={() => removeToken(index)}>
        <Icon name="close" size={10} />
      </button>
    </span>
  {/each}
  <input
    bind:this={inputRef}
    class="searchbar-input"
    value={draft}
    placeholder={tokens.length === 0 ? 'tag, namespace:value, -exclude — try "subject:" or "hero"' : ''}
    spellcheck="false"
    autocapitalize="off"
    autocomplete="off"
    aria-label="Search library"
    oninput={handleInput}
    onfocus={() => (open = Boolean(draft.trim()))}
    onkeydown={handleKeydown}
  />
  {#if tokens.length > 0 || draft}
    <button class="searchbar-clear" type="button" aria-label="Clear search" title="Clear search" onclick={clearAll}>
      <Icon name="close" size={13} />
    </button>
  {/if}

  {#if open && draft.trim() && flat.length > 0}
    <ul id="searchbar-suggestions" class="search-suggestions" role="listbox" aria-label="Search suggestions" onmousedown={(event) => event.preventDefault()}>
      {#each groups as group, groupIndex}
        {@const offset = groups.slice(0, groupIndex).reduce((sum, item) => sum + item.items.length, 0)}
        <li class="group-head">
          <span>{group.head}</span>
          <span>{group.items.length}</span>
        </li>
        {#each group.items as item, itemIndex}
          {@const flatIndex = offset + itemIndex}
          <li role="presentation">
            <button
              class:is-active={flatIndex === active}
              type="button"
              role="option"
              aria-selected={flatIndex === active}
              onmouseenter={() => (active = flatIndex)}
              onclick={() => selectSuggestion(item)}
            >
              <span class="tok">
                {#if item.ns}<span class="ns">{item.ns}:</span>{/if}<span>{item.val}</span>
              </span>
              {#if item.hint}<span class="hint">{item.hint}</span>{/if}
              {#if item.count != null}<span class="count">{item.count.toLocaleString()}</span>{/if}
            </button>
          </li>
        {/each}
      {/each}
      <li class="search-suggestions-footer">
        <span class="keys">
          <span><span class="g-kbd">↑</span><span class="g-kbd">↓</span> navigate</span>
          <span><span class="g-kbd">↵</span> select</span>
          <span><span class="g-kbd">Esc</span> close</span>
        </span>
        <span>prefix <span class="g-kbd">-</span> to exclude</span>
      </li>
    </ul>
  {/if}
</div>
