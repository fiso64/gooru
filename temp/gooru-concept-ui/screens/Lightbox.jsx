// Lightbox — left tag panel + centered stage + right rail of actions.

const {useState: _useSLB, useEffect: _useELB, useRef: _useRLB} = React;

function Lightbox({fileId, files, onClose, onPrev, onNext, onTagAdd, onTagRemove, iconStyle}) {
  const file = files.find(f => f.id === fileId);
  const [newTag, setNewTag] = _useSLB('');

  _useELB(() => {
    const h = (e) => {
      if (e.key === 'Escape') onClose();
      else if (e.key === 'ArrowLeft' || e.key === 'k') onPrev();
      else if (e.key === 'ArrowRight' || e.key === 'j') onNext();
    };
    window.addEventListener('keydown', h);
    return () => window.removeEventListener('keydown', h);
  }, [onClose, onPrev, onNext]);

  if (!file) return null;
  const grouped = groupTagsByNamespace(file.tags);
  const fileSize = file.sizeMB < 1 ? `${(file.sizeMB * 1000).toFixed(0)} KB` :
                   file.sizeMB < 1000 ? `${file.sizeMB.toFixed(1)} MB` : `${(file.sizeMB/1000).toFixed(2)} GB`;
  const modified = new Date(file.modified);
  const modifiedStr = modified.toLocaleString('en-GB', {year: 'numeric', month: 'short', day: '2-digit', hour: '2-digit', minute: '2-digit'});

  return (
    <div className="lightbox" onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      {/* Left: tags + meta */}
      <aside className="lightbox-aside">
        <div style={{display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8}}>
          <div className="g-eyebrow g-eyebrow-accent">{file.kind}</div>
          <button className="g-btn g-btn-ghost g-btn-sm g-btn-icon" onClick={onClose}><Icon name="close" size={14} style={iconStyle}/></button>
        </div>

        <h2 className="lightbox-name">{file.name}</h2>

        <dl className="lightbox-meta">
          <dt>Path</dt><dd style={{wordBreak: 'break-all'}}>{file.path}</dd>
          <dt>Size</dt><dd>{file.width}×{file.height} · {fileSize}</dd>
          {file.duration && <><dt>Length</dt><dd>{file.duration}</dd></>}
          <dt>Modified</dt><dd>{modifiedStr}</dd>
          <dt>Mime</dt><dd>{file.mime}</dd>
          <dt>Hash</dt><dd className="hash">{file.contentHash}</dd>
        </dl>

        <hr className="g-divider"/>

        <div className="lightbox-tags">
          <div className="lightbox-tag-group-head" style={{marginBottom: 4}}>
            <span>Tags · {file.tags.length}</span>
            <span style={{display: 'inline-flex', gap: 4}}>
              <button className="g-btn g-btn-ghost g-btn-sm g-btn-icon" title="History"><Icon name="info" size={13} style={iconStyle}/></button>
              <button className="g-btn g-btn-ghost g-btn-sm g-btn-icon" title="Suggest tags"><Icon name="sliders" size={13} style={iconStyle}/></button>
            </span>
          </div>

          {grouped.map(g => (
            <div key={g.ns || '__valueless'} className="lightbox-tag-group">
              {g.ns ? (
                <div className="lightbox-tag-group-head">
                  <span>{g.ns}</span>
                  <span>{g.tags.length}</span>
                </div>
              ) : null}
              <div className="lightbox-tag-list">
                {g.tags.map(t => (
                  <Tag key={t} tag={t} onRemove={() => onTagRemove(file.id, t)}/>
                ))}
              </div>
            </div>
          ))}

          <div className="lightbox-tag-input">
            <Icon name="plus" size={12} style={iconStyle}/>
            <input
              placeholder="add tag — e.g. subject:portrait"
              value={newTag}
              onChange={(e) => setNewTag(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && newTag.trim()) {
                  onTagAdd(file.id, newTag.trim());
                  setNewTag('');
                }
              }}
            />
          </div>
        </div>
      </aside>

      {/* Center: stage */}
      <div className="lightbox-stage">
        <img src={file.preview} alt={file.name}/>

        {/* video chrome overlay if video */}
        {file.kind === 'video' && (
          <div style={{
            position: 'absolute', left: '50%', bottom: 44, transform: 'translateX(-50%)',
            display: 'flex', alignItems: 'center', gap: 10,
            padding: '8px 12px',
            background: 'rgba(0,0,0,0.55)',
            border: '1px solid rgba(255,255,255,0.08)',
            backdropFilter: 'blur(20px)',
            WebkitBackdropFilter: 'blur(20px)',
            borderRadius: 999,
            color: '#fff',
            fontFamily: 'var(--font-mono)',
            fontSize: 11.5,
            minWidth: 360,
          }}>
            <Icon name="play" size={14} style="solid"/>
            <span style={{minWidth: 36, opacity: 0.85}}>0:00</span>
            <div style={{flex: 1, height: 3, background: 'rgba(255,255,255,0.15)', borderRadius: 2, overflow: 'hidden'}}>
              <div style={{width: '0%', height: '100%', background: 'var(--accent)'}}/>
            </div>
            <span style={{minWidth: 36, opacity: 0.85}}>{file.duration}</span>
          </div>
        )}

        <button className="lightbox-nav-arrow prev" onClick={onPrev} title="Previous (←)">
          <Icon name="chev_left" size={20} style={iconStyle}/>
        </button>
        <button className="lightbox-nav-arrow next" onClick={onNext} title="Next (→)">
          <Icon name="chev_right" size={20} style={iconStyle}/>
        </button>
      </div>

      {/* Right rail */}
      <aside className="lightbox-rail">
        <button className="g-btn g-btn-ghost" title="Add tag"><Icon name="tag" size={16} style={iconStyle}/></button>
        <button className="g-btn g-btn-ghost" title="Download original"><Icon name="download" size={16} style={iconStyle}/></button>
        <button className="g-btn g-btn-ghost" title="Open original in new tab"><Icon name="external" size={16} style={iconStyle}/></button>
        <div style={{flex: 1}}/>
        <button className="g-btn g-btn-ghost" title="Info"><Icon name="info" size={16} style={iconStyle}/></button>
        <button className="g-btn g-btn-ghost" title="Remove from library"><Icon name="trash" size={16} style={iconStyle}/></button>
      </aside>
    </div>
  );
}

Object.assign(window, {Lightbox});
