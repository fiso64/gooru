import React from 'react';

// Library screen — clean gallery grid, no inline metadata.
// Selection bar appears when any file is selected.

const {useState: _useSL, useMemo: _useML, useRef: _useRL, useEffect: _useEL, useCallback: _useCBL} = React;

function Library({iconStyle, density, layoutStyle, tokens, activeKind, onOpenFile, selection, setSelection}) {
  // Filter files: token-based search (from SearchBar) + the sidebar kind facet
  const filtered = _useML(() => {
    let arr = filterFilesByTokens(FILES, tokens);
    if (activeKind) arr = arr.filter(f => f.kind === activeKind);
    return arr;
  }, [tokens, activeKind]);

  const toggleSelect = (id, e) => {
    setSelection(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };
  const clearSelection = () => setSelection(new Set());
  const selectAll = () => setSelection(new Set(filtered.map(f => f.id)));

  const isFiltered = tokens.length > 0 || activeKind;

  return (
    <div className="main">
      {selection.size > 0 && (
        <SelectionBar
          count={selection.size}
          total={filtered.length}
          onClear={clearSelection}
          onSelectAll={selectAll}
          iconStyle={iconStyle}
        />
      )}
      <div className="library-head">
        <div className="library-head-title">
          <h1>Library</h1>
          <span className="library-head-meta">
            {filtered.length.toLocaleString()} file{filtered.length === 1 ? '' : 's'}
            {isFiltered && ` · filtered from ${FILES.length.toLocaleString()}`}
          </span>
        </div>
        <div className="library-head-actions">
          <Segmented value="modified" onChange={() => {}} options={[
            {value: 'modified', label: 'Modified'},
            {value: 'name', label: 'Name'},
            {value: 'size', label: 'Size'},
          ]}/>
          <button className="g-btn g-btn-sm" title="Sort direction">
            <Icon name="sort" size={14} style={iconStyle}/>
            <span>Newest</span>
          </button>
        </div>
      </div>

      {filtered.length === 0 ? (
        <EmptyState filtered={isFiltered} iconStyle={iconStyle}/>
      ) : (
        <div
          className={`grid layout-${layoutStyle}`}
          style={(() => {
            // Numeric density: free pixel value coming from Settings → Appearance.
            // Gap and outer padding scale with density so very tight grids feel
            // dense and huge thumbs get room to breathe.
            const px = Math.max(50, Math.min(500, Number(density) || 180));
            return {
              '--grid-cell': px + 'px',
              '--grid-gap': Math.max(2, Math.min(20, Math.round(px / 40))) + 'px',
              '--grid-pad': Math.max(12, Math.min(32, Math.round(px / 11))) + 'px',
            };
          })()}
        >
          {filtered.map(f => (
            <Thumb key={f.id}
                   file={f}
                   selected={selection.has(f.id)}
                   onClick={(id) => onOpenFile(id)}
                   onToggleSelect={toggleSelect}
                   iconStyle={iconStyle}/>
          ))}
          {/* infinite-scroll sentinel */}
          <div style={{
            gridColumn: '1 / -1',
            padding: '40px 0 8px',
            textAlign: 'center',
            color: 'var(--text-4)',
            fontFamily: 'var(--font-mono)',
            fontSize: 11,
            letterSpacing: '0.1em',
            textTransform: 'uppercase',
          }}>
            <span style={{display: 'inline-flex', alignItems: 'center', gap: 8}}>
              <span style={{
                display: 'inline-block', width: 10, height: 10, borderRadius: '50%',
                border: '1.5px solid var(--text-4)',
                borderTopColor: 'transparent',
                animation: 'g-spin 0.9s linear infinite',
              }}/>
              Loading more
            </span>
          </div>
        </div>
      )}
    </div>
  );
}

function SelectionBar({count, total, onClear, onSelectAll, iconStyle}) {
  return (
    <div className="selection-bar">
      <div style={{display: 'flex', alignItems: 'center', gap: 12}}>
        <Icon name="check" size={14} style="solid"/>
        <span><b style={{fontVariantNumeric: 'tabular-nums'}}>{count}</b> of <span style={{fontVariantNumeric: 'tabular-nums'}}>{total}</span> selected</span>
        {count < total && <button className="g-btn g-btn-sm" onClick={onSelectAll}>Select all {total}</button>}
      </div>
      <div className="sb-actions">
        <button className="g-btn g-btn-sm"><Icon name="tag" size={13} style={iconStyle}/> Tag…</button>
        <button className="g-btn g-btn-sm"><Icon name="download" size={13} style={iconStyle}/> Export</button>
        <button className="g-btn g-btn-sm"><Icon name="trash" size={13} style={iconStyle}/> Untag…</button>
        <button className="g-btn g-btn-sm g-btn-icon" onClick={onClear} title="Clear"><Icon name="close" size={13} style={iconStyle}/></button>
      </div>
    </div>
  );
}

function EmptyState({filtered, iconStyle}) {
  return (
    <div style={{
      flex: 1,
      display: 'grid', placeItems: 'center',
      padding: 80, textAlign: 'center',
    }}>
      <div style={{maxWidth: 380}}>
        <div style={{width: 60, height: 60, margin: '0 auto 16px', display: 'grid', placeItems: 'center',
                     background: 'var(--surface-2)', borderRadius: '50%', color: 'var(--text-3)'}}>
          <Icon name="search" size={24} style={iconStyle}/>
        </div>
        <h2 className="g-display" style={{fontSize: 22, margin: 0}}>No results</h2>
        <p style={{color: 'var(--text-3)', marginTop: 8}}>
          {filtered ? <>Nothing matches your filters. Try removing a pill, or check the tag spelling.</>
                    : <>Your library is empty. Drag files in, or run <code style={{color: 'var(--text)'}}>gooru import</code> from a terminal.</>}
        </p>
      </div>
    </div>
  );
}

Object.assign(window, {Library});
