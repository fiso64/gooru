// SearchBar — inline pills + IDE-style autocomplete dropdown.
//
// A "token" is one of:
//   namespace:value   →   { ns: 'subject', val: 'portrait', neg: false }
//   -namespace:value  →   { ns: 'subject', val: 'portrait', neg: true  }
//   @flag             →   { ns: '@',       val: 'favorite', neg: false }
//   bare-word         →   { ns: '',        val: 'roll',     neg: false }
//
// Tokens commit on Enter / Tab / suggestion click. Backspace on an empty
// input removes the last pill. Suggestions update as you type and adapt
// to whether you've typed past a `:` (then we suggest values within that
// namespace).

const {useState: _useSS, useRef: _useRS, useEffect: _useES, useMemo: _useMS} = React;

function parseToken(s) {
  let neg = false;
  if (s.startsWith('-')) { neg = true; s = s.slice(1); }
  const i = s.indexOf(':');
  if (i < 0) return {ns: '', val: s, neg};
  return {ns: s.slice(0, i), val: s.slice(i + 1), neg};
}
function tokenToString(t) {
  const body = t.ns ? t.ns + ':' + t.val : t.val;
  return (t.neg ? '-' : '') + body;
}

// Stable list of known namespaces (with totals)
function buildNamespaceCounts() {
  const map = {};
  TAG_INDEX.forEach(({tag, count}) => {
    const {ns} = parseTag(tag);
    if (!ns) return;
    map[ns] = (map[ns] || 0) + count;
  });
  return Object.entries(map).sort((a, b) => b[1] - a[1]).map(([ns, count]) => ({ns, count}));
}
const ALL_NAMESPACES = buildNamespaceCounts();
const VALUELESS_TAGS = TAG_INDEX.filter(t => !t.tag.includes(':'));

function highlight(text, term) {
  if (!term) return text;
  const i = text.toLowerCase().indexOf(term.toLowerCase());
  if (i < 0) return text;
  return (
    <>{text.slice(0, i)}<span className="match">{text.slice(i, i + term.length)}</span>{text.slice(i + term.length)}</>
  );
}

function computeSuggestions(draft, tokens) {
  const takenStrings = new Set(tokens.map(tokenToString));
  const takenBareValues = new Set(tokens.filter(t => !t.neg).map(t => `${t.ns}:${t.val}`));
  const draftRaw = draft;
  let working = draft;
  let negPrefix = '';
  if (working.startsWith('-')) { negPrefix = '-'; working = working.slice(1); }

  // CASE A — no colon yet: suggest namespaces + tag matches (value'd or valueless)
  if (!working.includes(':')) {
    const term = working.toLowerCase();
    const groups = [];

    // Namespace suggestions
    const nses = ALL_NAMESPACES
      .filter(({ns}) => !term || ns.toLowerCase().startsWith(term))
      .slice(0, 6)
      .map(({ns, count}) => ({
        kind: 'namespace',
        commit: negPrefix + ns + ':',
        label: <><span className="ns">{highlight(ns, term)}:</span></>,
        hint: 'namespace',
        count,
        partial: true, // selecting this only fills the draft; doesn't commit yet
      }));
    if (nses.length) groups.push({head: 'Namespaces', items: nses});

    // Tag matches — both value'd (subject:portrait) and valueless (review)
    if (term.length >= 1) {
      const tags = TAG_INDEX
        .filter(({tag}) => {
          if (takenBareValues.has(tag)) return false;
          const {value} = parseTag(tag);
          return value.toLowerCase().includes(term);
        })
        .slice(0, 8)
        .map(({tag, count}) => {
          const {ns, value} = parseTag(tag);
          return {
            kind: ns ? 'tag' : 'valueless',
            commit: negPrefix + tag,
            label: ns
              ? <><span className="ns">{ns}:</span>{highlight(value, term)}</>
              : <span>{highlight(value, term)}</span>,
            count,
          };
        });
      if (tags.length) groups.push({head: 'Tags', items: tags});
    } else {
      // Empty draft: surface the most-used valueless tags as a quick-pick
      const vs = VALUELESS_TAGS.filter(t => !takenStrings.has(t.tag)).slice(0, 5).map(({tag, count}) => ({
        kind: 'valueless',
        commit: negPrefix + tag,
        label: <span>{tag}</span>,
        count,
      }));
      if (vs.length) groups.push({head: 'Valueless', items: vs});
    }

    return groups;
  }

  // CASE B — colon present: suggest values inside that namespace
  const ci = working.indexOf(':');
  const ns = working.slice(0, ci).toLowerCase();
  const valFrag = working.slice(ci + 1).toLowerCase();

  const tags = TAG_INDEX
    .filter(({tag}) => {
      if (tag.startsWith('@')) return false;
      const p = parseTag(tag);
      if (p.ns.toLowerCase() !== ns) return false;
      if (takenBareValues.has(tag)) return false;
      if (valFrag && !p.value.toLowerCase().includes(valFrag)) return false;
      return true;
    })
    .slice(0, 12)
    .map(({tag, count}) => {
      const p = parseTag(tag);
      return {
        kind: 'value',
        commit: negPrefix + tag,
        label: <><span className="ns">{p.ns}:</span>{highlight(p.value, valFrag)}</>,
        count,
      };
    });

  return tags.length ? [{head: `${ns}:`, items: tags}] : [];
}

function flattenSuggestions(groups) {
  const arr = [];
  groups.forEach(g => g.items.forEach(it => arr.push({...it, group: g.head})));
  return arr;
}

function SearchBarPill({token, onRemove}) {
  return (
    <span className={"searchbar-pill" + (token.neg ? ' neg' : '')}>
      {token.neg && <span style={{opacity: 0.7}}>−</span>}
      {token.ns
        ? <><span className="ns">{token.ns}:</span><span>{token.val}</span></>
        : <span>{token.val}</span>}
      <button className="x" onClick={() => onRemove()} type="button" aria-label="Remove">
        <Icon name="close" size={10}/>
      </button>
    </span>
  );
}

function SearchBar({tokens, setTokens, iconStyle, placeholder}) {
  const [draft, setDraft] = _useSS('');
  const [open, setOpen] = _useSS(false);
  const [active, setActive] = _useSS(0);
  const inputRef = _useRS(null);
  const rootRef = _useRS(null);

  // Global `/` to focus
  _useES(() => {
    const h = (e) => {
      if (e.key === '/' && document.activeElement && document.activeElement.tagName !== 'INPUT') {
        e.preventDefault();
        inputRef.current && inputRef.current.focus();
      }
    };
    window.addEventListener('keydown', h);
    return () => window.removeEventListener('keydown', h);
  }, []);

  // Close dropdown on outside click
  _useES(() => {
    if (!open) return;
    const h = (e) => {
      if (rootRef.current && !rootRef.current.contains(e.target)) setOpen(false);
    };
    document.addEventListener('mousedown', h);
    return () => document.removeEventListener('mousedown', h);
  }, [open]);

  const groups = _useMS(() => computeSuggestions(draft, tokens), [draft, tokens]);
  const flat = _useMS(() => flattenSuggestions(groups), [groups]);

  // Keep active index in bounds
  _useES(() => { if (active >= flat.length) setActive(0); }, [flat.length]);

  const commitString = (s) => {
    s = s.trim();
    if (!s) return;
    const t = parseToken(s);
    if (!t.val && !t.ns) return;
    // Dedupe by serialised form
    const key = tokenToString(t);
    if (tokens.some(x => tokenToString(x) === key)) return;
    setTokens([...tokens, t]);
    setDraft('');
    setActive(0);
  };

  const onSuggestionSelect = (sugg) => {
    if (sugg.partial) {
      // Namespace completion — fill draft, keep open
      setDraft(sugg.commit);
      setActive(0);
      inputRef.current && inputRef.current.focus();
      return;
    }
    commitString(sugg.commit);
  };

  const onKeyDown = (e) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setActive(a => Math.min(a + 1, flat.length - 1));
      setOpen(true);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActive(a => Math.max(a - 1, 0));
      setOpen(true);
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      if (open && flat[active]) {
        e.preventDefault();
        onSuggestionSelect(flat[active]);
      } else if (draft.trim()) {
        e.preventDefault();
        commitString(draft);
      }
    } else if (e.key === 'Escape') {
      setOpen(false);
      setDraft('');
    } else if (e.key === 'Backspace' && draft === '' && tokens.length > 0) {
      e.preventDefault();
      setTokens(tokens.slice(0, -1));
    }
  };

  const removeToken = (i) => setTokens(tokens.filter((_, idx) => idx !== i));
  const clearAll = () => { setTokens([]); setDraft(''); setActive(0); };

  return (
    <div className="searchbar" ref={rootRef}
         onClick={() => inputRef.current && inputRef.current.focus()}>
      <span className="searchbar-icon"><Icon name="search" size={15} style={iconStyle}/></span>
      {tokens.map((t, i) => (
        <SearchBarPill key={i} token={t} onRemove={() => removeToken(i)}/>
      ))}
      <input
        ref={inputRef}
        className="searchbar-input"
        placeholder={tokens.length === 0 ? (placeholder || 'tag, namespace:value, -exclude — try "subject:" or "hero"') : ''}
        value={draft}
        spellCheck="false"
        autoCapitalize="off"
        autoComplete="off"
        onChange={(e) => { setDraft(e.target.value); setOpen(true); setActive(0); }}
        onFocus={() => setOpen(true)}
        onKeyDown={onKeyDown}
      />
      {(tokens.length > 0 || draft) && (
        <button className="searchbar-clear" type="button" onClick={(e) => {e.stopPropagation(); clearAll();}} title="Clear search">
          <Icon name="close" size={13} style={iconStyle}/>
        </button>
      )}

      {open && flat.length > 0 && (
        <ul className="search-suggestions" onMouseDown={(e) => e.preventDefault()}>
          {groups.map((g, gi) => {
            const offset = groups.slice(0, gi).reduce((a, x) => a + x.items.length, 0);
            return (
              <React.Fragment key={g.head + gi}>
                <li className="group-head" style={{cursor: 'default'}}>
                  <span>{g.head}</span>
                  <span>{g.items.length}</span>
                </li>
                {g.items.map((it, i) => {
                  const flatIdx = offset + i;
                  return (
                    <li key={it.commit + i}
                        className={flatIdx === active ? 'is-active' : ''}
                        onMouseEnter={() => setActive(flatIdx)}
                        onClick={() => onSuggestionSelect(it)}>
                      <span className="tok">{it.label}</span>
                      {it.hint && <span className="hint">{it.hint}</span>}
                      {it.count != null && <span className="count">{it.count}</span>}
                    </li>
                  );
                })}
              </React.Fragment>
            );
          })}
          <div className="search-suggestions-footer">
            <span className="keys">
              <span><span className="g-kbd">↑</span><span className="g-kbd">↓</span> navigate</span>
              <span><span className="g-kbd">↵</span> select</span>
              <span><span className="g-kbd">Esc</span> close</span>
            </span>
            <span>prefix <span className="g-kbd">−</span> to exclude</span>
          </div>
        </ul>
      )}
    </div>
  );
}

// Token-based filter — used by Library
function filterFilesByTokens(files, tokens) {
  if (!tokens.length) return files;
  return files.filter(f => {
    for (const t of tokens) {
      let matches;
      if (t.ns) {
        matches = f.tags.includes(`${t.ns}:${t.val}`);
      } else {
        // valueless: must match an exact valueless tag, or substring of any tag / name
        matches = f.tags.includes(t.val) || f.name.toLowerCase().includes(t.val.toLowerCase()) || f.tags.some(tag => tag.toLowerCase().includes(t.val.toLowerCase()));
      }
      if (t.neg ? matches : !matches) return false;
    }
    return true;
  });
}

Object.assign(window, {SearchBar, filterFilesByTokens, parseToken, tokenToString});
