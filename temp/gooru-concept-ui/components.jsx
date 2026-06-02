import React from 'react';

// Gooru — reusable UI pieces

const {useState, useEffect, useRef, useMemo, useCallback} = React;

// -- Tag --------------------------------------------------------------
function Tag({tag, onClick, onRemove, active = false}) {
  const {ns, value} = parseTag(tag);
  return (
    <span
      className={"g-tag" + (active ? " g-tag-active" : "")}
      onClick={onClick}
      role={onClick ? "button" : undefined}
      tabIndex={onClick ? 0 : undefined}
    >
      {ns && <span className="g-tag-ns">{ns}:</span>}
      <span>{value}</span>
      {onRemove && (
        <button
          className="g-tag-x"
          onClick={(e) => {e.stopPropagation(); onRemove(tag);}}
          style={{background: 'transparent', border: 0, padding: 0, marginLeft: 2, color: 'inherit', cursor: 'pointer', display: 'inline-flex'}}
          aria-label={`Remove ${tag}`}
        >
          <Icon name="close" size={11}/>
        </button>
      )}
    </span>
  );
}

// -- Thumbnail card --------------------------------------------------
function Thumb({file, selected, onClick, onToggleSelect, iconStyle = 'line'}) {
  return (
    <div
      className={"thumb" + (selected ? " is-selected" : "")}
      onClick={(e) => { if (e.shiftKey || e.metaKey || e.ctrlKey) { onToggleSelect && onToggleSelect(file.id, e); } else { onClick && onClick(file.id); } }}
      tabIndex={0}
    >
      <img src={file.thumb} alt={file.name} loading="lazy" draggable={false}/>
      <div className="thumb-overlay"/>
      <div className="thumb-badges">
        {file.kind === 'video' && (
          <span className="thumb-badge"><Icon name="play" size={9} style="solid"/> {file.duration}</span>
        )}
        {file.kind === 'gif' && (
          <span className="thumb-badge thumb-badge-gif">GIF · {file.duration}</span>
        )}
      </div>
      <div
        className="thumb-checkbox"
        onClick={(e) => {e.stopPropagation(); onToggleSelect && onToggleSelect(file.id, e);}}
        role="checkbox"
        aria-checked={selected}
      >
        {selected && <Icon name="check" size={12} style="solid"/>}
      </div>
      <div className="thumb-meta">
        <span className="thumb-meta-name">{file.name}</span>
        <span>{file.width}×{file.height}</span>
      </div>
    </div>
  );
}

// -- Sidebar nav item ------------------------------------------------
function NavItem({icon, label, count, active, iconStyle, onClick}) {
  return (
    <div className={"sidebar-item" + (active ? " is-active" : "")} onClick={onClick} role="button" tabIndex={0}>
      <Icon name={icon} size={16} style={iconStyle} active={active}/>
      <span>{label}</span>
      {count != null && <span className="count">{count.toLocaleString()}</span>}
    </div>
  );
}

// -- Sidebar ---------------------------------------------------------
function Sidebar({route, setRoute, iconStyle, activeKind, toggleKind, activeSavedSearch, runSavedSearch}) {
  return (
    <aside className="sidebar">
      <div className="sidebar-section">
        <NavItem icon="library" label="Library" iconStyle={iconStyle}
                 active={route === 'library'} onClick={() => setRoute('library')}
                 count={FILES.length}/>
        <NavItem icon="tags" label="Tags" iconStyle={iconStyle}
                 active={route === 'tags'} onClick={() => setRoute('tags')}
                 count={TAG_INDEX.length}/>
        <NavItem icon="upload" label="Upload" iconStyle={iconStyle}
                 active={route === 'upload'} onClick={() => setRoute('upload')}/>
        <NavItem icon="jobs" label="Jobs" iconStyle={iconStyle}
                 active={route === 'jobs'} onClick={() => setRoute('jobs')}
                 count={JOBS.filter(j => j.status === 'running').length}/>
      </div>

      <div className="sidebar-section">
        <div className="sidebar-section-head">Kinds</div>
        {KINDS.map(k => (
          <NavItem key={k.key}
                   icon={k.key === 'photo' ? 'photo' : k.key === 'video' ? 'video' : 'gif'}
                   label={k.label} count={k.count} iconStyle={iconStyle}
                   active={route === 'library' && activeKind === k.key}
                   onClick={() => toggleKind(k.key)}/>
        ))}
      </div>

      <div className="sidebar-section">
        <div className="sidebar-section-head">Saved searches</div>
        {SAVED_SEARCHES.map(s => {
          const isActive = route === 'library' && activeSavedSearch === s.name;
          return (
            <div key={s.name} className={"sidebar-item" + (isActive ? ' is-active' : '')}
                 onClick={() => runSavedSearch(s)}>
              <Icon name="bookmark" size={14} style={iconStyle} active={isActive}/>
              <span style={{overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap'}}>{s.name}</span>
            </div>
          );
        })}
      </div>

      <div className="sidebar-section" style={{marginTop: 'auto'}}>
        <NavItem icon="settings" label="Settings" iconStyle={iconStyle}
                 active={route === 'settings'} onClick={() => setRoute('settings')}/>
        <NavItem icon="keyboard" label="Shortcuts" iconStyle={iconStyle}
                 active={route === 'shortcuts'} onClick={() => setRoute('shortcuts')}/>
      </div>
    </aside>
  );
}

// -- TopBar ----------------------------------------------------------
function TopBar({tokens, setTokens, iconStyle, logoVariant, logoParams, onJobs, onUser, jobsActiveCount, user}) {
  const wordmark = logoIsWordmark(logoVariant);
  return (
    <header className="topbar">
      <div className="topbar-brand">
        <span className="topbar-brand-mark">
          <Logo variant={logoVariant} size={wordmark ? 17 : 26} color="var(--accent)" {...(logoParams || {})}/>
        </span>
        {!wordmark && <span className="topbar-brand-name">gooru</span>}
      </div>
      <div className="topbar-search">
        <div className="topbar-search-inner">
          <SearchBar tokens={tokens} setTokens={setTokens} iconStyle={iconStyle}/>
        </div>
      </div>
      <div className="topbar-right">
        <button className="g-btn g-btn-ghost g-btn-sm g-btn-icon" title="Jobs" onClick={onJobs}>
          <Icon name="jobs" size={16} style={iconStyle}/>
          {jobsActiveCount > 0 && <span style={{
            display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
            background: 'var(--accent)', color: 'var(--accent-ink)',
            fontFamily: 'var(--font-mono)', fontSize: 10, fontWeight: 600,
            minWidth: 16, height: 16, padding: '0 4px', borderRadius: 8,
            marginLeft: 2,
          }}>{jobsActiveCount}</span>}
        </button>
        <button className="g-btn g-btn-ghost g-btn-sm" title={user} onClick={onUser}>
          <Icon name="user" size={14} style={iconStyle}/>
          <span style={{fontSize: 12}}>{user}</span>
        </button>
      </div>
    </header>
  );
}

// -- Facets bar (above grid) -----------------------------------------
function FacetsBar({activeFacets, setActiveFacets, query, iconStyle}) {
  const chips = useMemo(() => {
    const list = [];
    if (activeFacets.kind) list.push({k: 'kind', v: activeFacets.kind, label: activeFacets.kind});
    (activeFacets.tags || []).forEach(t => list.push({k: 'tag', v: t, label: t}));
    if (query) list.push({k: 'query', v: query, label: `"${query}"`});
    return list;
  }, [activeFacets, query]);

  const removeChip = (c) => {
    if (c.k === 'kind') setActiveFacets({...activeFacets, kind: null});
    if (c.k === 'tag')  setActiveFacets({...activeFacets, tags: activeFacets.tags.filter(t => t !== c.v)});
    if (c.k === 'query') setActiveFacets({...activeFacets, _clearQuery: Date.now()});
  };

  if (chips.length === 0) return null;
  return (
    <div className="facets-bar">
      <span className="g-eyebrow" style={{flex: '0 0 auto'}}>Filtering</span>
      {chips.map((c, i) => (
        <span key={i} className="facet-chip is-active" onClick={() => removeChip(c)}>
          <span style={{fontFamily: 'var(--font-mono)'}}>{c.label}</span>
          <span className="x"><Icon name="close" size={10} style={iconStyle}/></span>
        </span>
      ))}
      <button className="g-btn g-btn-ghost g-btn-sm" style={{marginLeft: 'auto'}}
              onClick={() => setActiveFacets({tags: [], kind: null, _clearQuery: Date.now()})}>
        Clear all
      </button>
    </div>
  );
}

// -- Segmented control -----------------------------------------------
function Segmented({value, onChange, options}) {
  return (
    <div className="seg">
      {options.map(o => (
        <button key={o.value}
                className={value === o.value ? "is-active" : ""}
                onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  );
}

// expose
Object.assign(window, {Tag, Thumb, NavItem, Sidebar, TopBar, FacetsBar, Segmented});
