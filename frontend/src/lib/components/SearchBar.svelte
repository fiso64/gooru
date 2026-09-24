<script lang="ts">
  import { tick } from 'svelte';
  import Icon from './Icon.svelte';
  import { specialSearchSuggestions } from '$lib/search/specialSuggestions';
  import { parseSearchQuery, parseSearchToken, searchTokensToQuery, searchTokenToString, type SearchToken } from '$lib/search/tokens';
  import { rankCompletionCandidates } from '$lib/utils/completionRanking';
  import { plainTagSuggestions } from '$lib/utils/tagSuggestions';
  import { hasCommandModifier, matchesShortcutModifiers, searchShortcutAction } from '$lib/utils/keyboard';
  import { keepActiveCompletionVisible } from '$lib/utils/completionVisibility';
  import { moveCompletionIndex } from '$lib/utils/completionNavigation';

  type TagLike = { name?: string; tag?: string; namespace?: string; value?: string; count?: number };
  type SuggestionLike = { name: string; value?: string; count?: number };
  type MetaTagLike = { syntax: string; hint: string; requires_value: boolean };
  type SearchPresentation = 'tokens' | 'text';
  type SuggestionItem = {
    kind: 'namespace' | 'tag' | 'valueless' | 'value' | 'special';
    commit: string;
    ns: string;
    val: string;
    count?: number;
    hint?: string;
    partial?: boolean;
  };
  type SuggestionGroup = { items: SuggestionItem[] };

  const defaultPlaceholder = 'tag, namespace:value, -exclude, @metatag';

  let {
    value,
    suggestions,
    metaTags,
    tags,
    onDraftInput,
    onCommit,
    presentation = 'tokens',
    placeholder = defaultPlaceholder,
    showShortcutHint = true,
    enableSlashShortcut = true
  } = $props<{
    value: string;
    suggestions: SuggestionLike[];
    metaTags: MetaTagLike[];
    tags: TagLike[];
    onDraftInput: (value: string) => void;
    onCommit: (value: string) => void;
    presentation?: SearchPresentation;
    placeholder?: string;
    showShortcutHint?: boolean;
    enableSlashShortcut?: boolean;
  }>();

  let tokens = $state<SearchToken[]>([]);
  let draft = $state('');
  let open = $state(false);
  let active = $state(0);
  let inputRef = $state<HTMLInputElement | undefined>();
  let rootRef = $state<HTMLDivElement | undefined>();
  let suggestionsRef = $state<HTMLUListElement | undefined>();
  let lastSyncedValue = $state('');

  const textMode = $derived(presentation === 'text');
  const tagItems = $derived(normalizeTags(tags, suggestions));
  const completionContext = $derived(textMode ? plainTextCompletionContext(draft) : { prefix: '', fragment: draft, tokens });
  const groups = $derived(computeSuggestions(completionContext.fragment, completionContext.tokens, tagItems, metaTags));
  const flat = $derived(groups.flatMap((group) => group.items));

  $effect(() => {
    if (value === lastSyncedValue) return;
    if (textMode) {
      tokens = [];
      draft = value;
    } else {
      tokens = parseSearchQuery(value);
      draft = '';
    }
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
      if (!action || (!enableSlashShortcut && action === 'focus-search')) return;
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

  function plainTextCompletionContext(input: string) {
    const match = input.match(/^(.*\s)?([^\s]*)$/);
    const prefix = match?.[1] ?? '';
    const fragment = match?.[2] ?? input;
    return {
      prefix,
      fragment,
      tokens: parseSearchQuery(prefix.trim())
    };
  }

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
      if (completionItems.length) result.push({ items: completionItems });
      if (specialItems.length) result.push({ items: specialItems });
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
    if (values.length) result.push({ items: values });
    if (specialItems.length) result.push({ items: specialItems });
    return result;
  }

  function syncCommit(nextTokens: SearchToken[]) {
    tokens = nextTokens;
    const query = searchTokensToQuery(tokens);
    lastSyncedValue = query;
    onCommit(query);
  }

  function commitPlainText(input: string, keepTrailingSpace = false) {
    const query = input.trim();
    draft = keepTrailingSpace && query ? `${query} ` : query;
    open = false;
    active = 0;
    lastSyncedValue = query;
    onDraftInput('');
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
    if (textMode) {
      const context = plainTextCompletionContext(draft);
      const next = `${context.prefix}${item.commit}`;
      if (item.partial) {
        draft = next;
        active = 0;
        open = true;
        onDraftInput(item.commit);
        inputRef?.focus();
        return;
      }
      commitPlainText(next, true);
      inputRef?.focus();
      return;
    }

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
    if (textMode) {
      lastSyncedValue = '';
      onCommit('');
      return;
    }
    syncCommit([]);
  }

  function handleInput(event: Event) {
    draft = (event.currentTarget as HTMLInputElement).value;
    const fragment = textMode ? plainTextCompletionContext(draft).fragment : draft;
    open = Boolean(fragment.trim());
    active = 0;
    onDraftInput(fragment);
  }

  function handleKeydown(event: KeyboardEvent) {
    if (!matchesShortcutModifiers(event)) return;
    const fragment = textMode ? plainTextCompletionContext(draft).fragment : draft;
    if (event.key === 'ArrowDown') {
      if (!fragment.trim()) return;
      event.preventDefault();
      active = moveCompletionIndex(active, flat.length, 1);
      open = true;
      return;
    }
    if (event.key === 'ArrowUp') {
      if (!fragment.trim()) return;
      event.preventDefault();
      active = moveCompletionIndex(active, flat.length, -1);
      open = true;
      return;
    }
    if (event.key === 'Enter' || event.key === 'Tab') {
      if (open && flat[active]) {
        event.preventDefault();
        selectSuggestion(flat[active]);
      } else if (draft.trim() || (textMode && event.key === 'Enter')) {
        event.preventDefault();
        if (textMode) commitPlainText(draft);
        else commitString(draft);
      }
      return;
    }
    if (event.key === 'Escape') {
      event.preventDefault();
      open = false;
      onDraftInput('');
      if (!textMode) draft = '';
      inputRef?.blur();
      return;
    }
    if (!textMode && event.key === 'Backspace' && !draft && tokens.length > 0) {
      event.preventDefault();
      removeToken(tokens.length - 1);
    }
  }
</script>

<!-- Pointer clicks on the shell only forward focus to the nested keyboard-focusable input. -->
<!-- svelte-ignore a11y_click_events_have_key_events -->
<div
  class="searchbar"
  class:searchbar-text={textMode}
  role="combobox"
  aria-label={textMode ? 'Search query' : 'Search tokens'}
  aria-controls="searchbar-suggestions"
  aria-expanded={open && flat.length > 0 ? 'true' : 'false'}
  aria-haspopup="listbox"
  tabindex="-1"
  bind:this={rootRef}
  onclick={() => inputRef?.focus()}
>
  <span class="searchbar-icon"><Icon name="search" size={15} /></span>
  {#if !textMode}
    {#each tokens as token, index}
      <span class:neg={token.neg} class="searchbar-pill">
        {#if token.neg}<span class="neg-symbol">−</span>{/if}
        {#if token.ns}<span class="ns">{token.ns}:</span>{/if}<span>{token.val}</span>
        <button class="x" type="button" aria-label={`Remove ${searchTokenToString(token)}`} onclick={() => removeToken(index)}>
          <Icon name="close" size={10} />
        </button>
      </span>
    {/each}
  {/if}
  <input
    bind:this={inputRef}
    class="searchbar-input"
    value={draft}
    placeholder={textMode || tokens.length === 0 ? placeholder : ''}
    spellcheck="false"
    autocapitalize="off"
    autocomplete="off"
    aria-label="Search library"
    oninput={handleInput}
    onfocus={() => (open = Boolean((textMode ? plainTextCompletionContext(draft).fragment : draft).trim()))}
    onkeydown={handleKeydown}
  />
  {#if tokens.length > 0 || draft}
    <button class="searchbar-clear" type="button" aria-label="Clear search" title="Clear search" onclick={clearAll}>
      <Icon name="close" size={13} />
    </button>
  {:else if showShortcutHint}
    <span class="searchbar-shortcut g-kbd" aria-hidden="true">/</span>
  {/if}

  {#if open && completionContext.fragment.trim() && flat.length > 0}
    <ul bind:this={suggestionsRef} id="searchbar-suggestions" class="search-suggestions" role="listbox" aria-label="Search suggestions" onmousedown={(event) => event.preventDefault()}>
      {#each flat as item, flatIndex}
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
