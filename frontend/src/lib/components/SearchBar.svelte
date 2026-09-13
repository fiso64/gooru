<script lang="ts">
  import { tick } from 'svelte';
  import Icon from './Icon.svelte';
  import { specialSearchSuggestions } from '$lib/search/specialSuggestions';
  import { parseSearchQuery, parseSearchToken, searchTokensToQuery, searchTokenToString, type SearchToken } from '$lib/search/tokens';
  import { rankCompletionCandidates } from '$lib/utils/completionRanking';
  import { plainTagSuggestions } from '$lib/utils/tagSuggestions';
  import { hasCommandModifier, searchShortcutAction } from '$lib/utils/keyboard';
  import { keepActiveCompletionVisible } from '$lib/utils/completionVisibility';

  type TagLike = { name?: string; tag?: string; namespace?: string; value?: string; count?: number };
  type SuggestionLike = { name: string; value?: string; count?: number };
  type MetaTagLike = { syntax: string; hint: string; requires_value: boolean };
  type SuggestionItem = {
    kind: 'namespace' | 'tag' | 'valueless' | 'value' | 'special';
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
    metaTags,
    tags,
    onDraftInput,
    onCommit
  } = $props<{
    value: string;
    suggestions: SuggestionLike[];
    metaTags: MetaTagLike[];
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
  let suggestionsRef = $state<HTMLUListElement | undefined>();
  let lastSyncedValue = $state('');

  const tagItems = $derived(normalizeTags(tags, suggestions));
  const groups = $derived(computeSuggestions(draft, tokens, tagItems, metaTags));
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
    active;
    if (!open) return;
    void tick().then(() => keepActiveCompletionVisible(suggestionsRef));
  });

  function beginFilenameSearch() {
    const retainedTokens = tokens.filter((token) => token.ns.toLowerCase() !== '@filename_contains');
    if (retainedTokens.length !== tokens.length) syncCommit(retainedTokens);
    draft = '@filename_contains:';
    open = true;
    active = 0;
    onDraftInput(draft);
    void tick().then(() => {
      inputRef?.focus();
      inputRef?.setSelectionRange(draft.length, draft.length);
    });
  }

  $effect(() => {
    const handleKeydown = (event: KeyboardEvent) => {
      const action = searchShortcutAction(event.key, event.target, hasCommandModifier(event), event.shiftKey);
      if (!action) return;
      event.preventDefault();
      if (action === 'filename-search') beginFilenameSearch();
      else inputRef?.focus();
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
    const seen = new Map<string, { tag: string; count?: number }>();
    for (const tag of tags) {
      const name = tag.name ?? tag.tag ?? (tag.namespace ? `${tag.namespace}:${tag.value ?? ''}` : (tag.value ?? ''));
      if (!name || name.startsWith('@')) continue;
      seen.set(name, { tag: name, count: tag.count ?? seen.get(name)?.count });
    }
    for (const suggestion of suggestions) {
      let name = suggestion.name?.trim();
      if (!name) continue;
      if (name.startsWith('-@')) name = name.slice(1);
      if (name.startsWith('@')) {
        const colon = name.indexOf(':');
        if (colon < 0 || colon === name.length - 1) continue;
      }
      seen.set(name, { tag: name, count: suggestion.count ?? seen.get(name)?.count });
    }
    return [...seen.values()].sort((a, b) => (b.count ?? 0) - (a.count ?? 0) || a.tag.localeCompare(b.tag));
  }

  function parseTag(tag: string) {
    const colon = tag.indexOf(':');
    if (colon < 0) return { ns: '', value: tag };
    return { ns: tag.slice(0, colon), value: tag.slice(colon + 1) };
  }

  function computeSuggestions(
    draftValue: string,
    currentTokens: SearchToken[],
    items: Array<{ tag: string; count?: number }>,
    metaTags: MetaTagLike[]
  ): SuggestionGroup[] {
    const tokenStrings = new Set(currentTokens.map(searchTokenToString));
    const takenPositiveTags = new Set(
      currentTokens
        .filter((token) => !token.neg)
        .map((token) => (token.ns ? `${token.ns}:${token.val}` : token.val))
    );
    const specialItems: SuggestionItem[] = specialSearchSuggestions(draftValue, [...tokenStrings], metaTags).map((item) => ({
      kind: 'special',
      ...item
    }));
    let working = draftValue.trim();
    let negPrefix = '';
    if (working.startsWith('-')) {
      negPrefix = '-';
      working = working.slice(1);
    }

    if (!working.includes(':')) {
      const result: SuggestionGroup[] = [];
      const completionItems = plainTagSuggestions(
        working,
        items.map(({ tag, count }) => ({ name: tag, count })),
        [],
        14
      )
        .filter(({ name, kind }) => kind === 'namespace' || (!takenPositiveTags.has(name) && !tokenStrings.has(`${negPrefix}${name}`)))
        .map(({ name, count, kind }) => {
          if (kind === 'namespace') {
            const ns = name.slice(0, -1);
            return {
              kind: 'namespace' as const,
              commit: `${negPrefix}${name}`,
              ns,
              val: '',
              count,
              hint: 'namespace',
              partial: true
            };
          }
          const parsed = parseTag(name);
          return {
            kind: parsed.ns ? ('tag' as const) : ('valueless' as const),
            commit: `${negPrefix}${name}`,
            ns: parsed.ns,
            val: parsed.value,
            count
          };
        });
      if (completionItems.length) result.push({ head: 'Suggestions', items: completionItems });
      if (specialItems.length) result.push({ head: 'Query', items: specialItems });
      return result;
    }

    const colon = working.indexOf(':');
    const ns = working.slice(0, colon).toLowerCase();
    const valFrag = working.slice(colon + 1).toLowerCase();
    const valueCandidates = items
      .filter(({ tag }) => {
        if (takenPositiveTags.has(tag)) return false;
        const parsed = parseTag(tag);
        if (parsed.ns.toLowerCase() !== ns) return false;
        return !valFrag || parsed.value.toLowerCase().includes(valFrag);
      })
      .map(({ tag, count }) => ({ name: parseTag(tag).value, tag, count }));
    const values = rankCompletionCandidates(valueCandidates, valFrag)
      .slice(0, 12)
      .map(({ tag, count }) => {
        const parsed = parseTag(tag);
        return { kind: 'value' as const, commit: `${negPrefix}${tag}`, ns: parsed.ns, val: parsed.value, count };
      });

    const result: SuggestionGroup[] = [];
    if (values.length) result.push({ head: `${ns}:`, items: values });
    if (specialItems.length) result.push({ head: 'Query', items: specialItems });
    return result;
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
    placeholder={tokens.length === 0 ? 'tag, namespace:value, -exclude, @metatag' : ''}
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
  {:else}
    <span class="searchbar-shortcut g-kbd" aria-hidden="true">/</span>
  {/if}

  {#if open && draft.trim() && flat.length > 0}
    <ul bind:this={suggestionsRef} id="searchbar-suggestions" class="search-suggestions" role="listbox" aria-label="Search suggestions" onmousedown={(event) => event.preventDefault()}>
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

<style>
  .searchbar-shortcut {
    margin-left: auto;
    margin-right: 5px;
    flex: 0 0 auto;
    color: var(--text-3);
    font-size: 9px;
    pointer-events: none;
  }
</style>
