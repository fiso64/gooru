import React from 'react';

// Upload, Jobs (page + drawer), Tags index, Shortcuts, Settings — grouped because each is small.

const {useState: _useSP, useRef: _useRP, useMemo: _useMP, useEffect: _useEP} = React;

// ---- Upload ---------------------------------------------------------
function Upload({iconStyle, items, addFiles, autoUpload, setAutoUpload, startStagedUploads, clearStaged, removeItem}) {
  const [drag, setDrag] = _useSP(false);
  const fileInputRef = _useRP(null);
  const onChooseFiles = () => fileInputRef.current && fileInputRef.current.click();
  const onFilePicked = (e) => {
    if (addFiles) addFiles(e.target.files);
    e.target.value = '';
  };

  const fmtSize = (b) => b > 1e9 ? `${(b/1e9).toFixed(2)} GB` : b > 1e6 ? `${(b/1e6).toFixed(1)} MB` : b > 1e3 ? `${(b/1e3).toFixed(0)} KB` : `${b} B`;

  const stagedItems = items.filter(it => it.status === 'staged');
  const queueItems  = items.filter(it => it.status !== 'staged');
  const stagedSize  = stagedItems.reduce((a, b) => a + b.size, 0);

  // Pick the right thumbnail for a queue row: real image/video if we have an
  // object URL, otherwise fall back to the icon glyph for the file kind.
  const renderThumb = (it) => {
    if (it.url && it.kind === 'image') return <img src={it.url} alt=""/>;
    if (it.url && it.kind === 'video') return <video src={it.url} muted playsInline preload="metadata"/>;
    const glyph = it.kind === 'video' ? 'video'
                : /\.(zip|tar|gz)$/i.test(it.name) ? 'folder'
                : it.kind === 'other' ? 'photo'
                : 'photo';
    return <Icon name={glyph} size={18} style={iconStyle}/>;
  };

  return (
    <div className="main">
      <div className="page">
        <div className="page-header">
          <div className="g-eyebrow g-eyebrow-accent">Upload</div>
          <h1>Import media into your library</h1>
          <p>Files are content-hashed on receipt. Duplicates are detected automatically. Initial tags can be applied here.</p>
        </div>

        <div className="g-card" style={{padding: 18, display: 'flex', flexDirection: 'column', gap: 16}}>
          <div className="field-row">
            <label>Target</label>
            <select className="g-input" defaultValue="default">
              <option value="default">default — /Users/me/library</option>
              <option value="archive">archive — /Volumes/work/archive</option>
              <option value="inbox">inbox — /Users/me/library/inbox</option>
            </select>
          </div>
          <div className="field-row">
            <label>Initial tags</label>
            <div className="field-control" style={{display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center'}}>
              <Tag tag="collection:may-2026"/>
              <Tag tag="@review"/>
              <button className="g-btn g-btn-ghost g-btn-sm"><Icon name="plus" size={12} style={iconStyle}/> Add tag</button>
            </div>
          </div>
          <div className="field-row">
            <label>On conflict</label>
            <Segmented value="rename" onChange={() => {}} options={[
              {value: 'skip', label: 'Skip'},
              {value: 'rename', label: 'Rename'},
              {value: 'replace', label: 'Replace'},
            ]}/>
          </div>
          <div className="field-row">
            <label>On drop</label>
            <Segmented
              value={autoUpload ? 'auto' : 'stage'}
              onChange={(v) => setAutoUpload && setAutoUpload(v === 'auto')}
              options={[
                {value: 'stage', label: 'Stage first'},
                {value: 'auto',  label: 'Auto-upload'},
              ]}
            />
          </div>
        </div>

        <div
          className={"upload-zone" + (drag ? ' is-drag' : '')}
          onClick={onChooseFiles}
          onDragOver={(e) => {e.preventDefault(); setDrag(true);}}
          onDragLeave={() => setDrag(false)}
          onDrop={(e) => {e.preventDefault(); setDrag(false);}}
        >
          <input ref={fileInputRef} type="file" multiple style={{display: 'none'}} onChange={onFilePicked}/>
          <div className="icon-wrap"><Icon name="upload" size={26} style={iconStyle}/></div>
          <h3>Drop files here</h3>
          <p>or click to browse · max 5 GB per file</p>
          <div style={{display: 'flex', gap: 8}}>
            <button className="g-btn g-btn-primary" onClick={(e) => {e.stopPropagation(); onChooseFiles();}}><Icon name="folder" size={14} style={iconStyle}/> Choose files…</button>
            <button className="g-btn" onClick={(e) => e.stopPropagation()}><Icon name="external" size={14} style={iconStyle}/> Paste URL</button>
          </div>
        </div>

        {stagedItems.length > 0 && (
          <div>
            <div style={{display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 10}}>
              <div className="g-eyebrow">Staged · {stagedItems.length} {stagedItems.length === 1 ? 'file' : 'files'} · {fmtSize(stagedSize)}</div>
              <div style={{display: 'flex', gap: 6}}>
                <button className="g-btn g-btn-sm" onClick={() => clearStaged && clearStaged()}>
                  <Icon name="close" size={12} style={iconStyle}/> Clear staged
                </button>
                <button className="g-btn g-btn-primary g-btn-sm" onClick={() => startStagedUploads && startStagedUploads()}>
                  <Icon name="upload" size={12} style={iconStyle}/> Upload {stagedItems.length} {stagedItems.length === 1 ? 'file' : 'files'}
                </button>
              </div>
            </div>
            <div className="g-card" style={{overflow: 'hidden'}}>
              <div className="upload-list">
                {stagedItems.map((it) => (
                  <div key={it.id} className="upload-row upload-row-staged">
                    <div className="thumb-tile">{renderThumb(it)}</div>
                    <div>
                      <div className="name">{it.name}</div>
                    </div>
                    <div className="size">{fmtSize(it.size)}</div>
                    <div className="progress is-staged" aria-hidden="true"/>
                    <div className="status">
                      <button
                        className="g-btn g-btn-ghost g-btn-sm g-btn-icon"
                        title="Remove from staging"
                        onClick={() => removeItem && removeItem(it.id)}
                      >
                        <Icon name="close" size={11} style={iconStyle}/>
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}

        <div>
          <div style={{display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 10}}>
            <div className="g-eyebrow">Queue · {queueItems.length} files · {fmtSize(queueItems.reduce((a, b) => a + b.size, 0))}</div>
            <div style={{display: 'flex', gap: 6}}>
              <button className="g-btn g-btn-sm"><Icon name="pause" size={12} style={iconStyle}/> Pause all</button>
              <button className="g-btn g-btn-sm"><Icon name="close" size={12} style={iconStyle}/> Clear done</button>
            </div>
          </div>
          <div className="g-card" style={{overflow: 'hidden'}}>
            <div className="upload-list">
              {queueItems.map((it) => (
                <div key={it.id} className="upload-row">
                  <div className="thumb-tile">{renderThumb(it)}</div>
                  <div>
                    <div className="name">{it.name}</div>
                    {it.error && <div style={{color: 'var(--danger)', fontFamily: 'var(--font-mono)', fontSize: 11, marginTop: 2}}>{it.error}</div>}
                  </div>
                  <div className="size">{fmtSize(it.size)}</div>
                  <div className="progress"><div style={{width: it.progress + '%', background: it.status === 'done' ? 'var(--ok)' : it.status === 'skipped' ? 'var(--text-3)' : 'var(--accent)'}}/></div>
                  <div className={"status " + (it.status === 'done' ? 'ok' : it.status === 'error' ? 'err' : '')}>
                    {it.status === 'uploading' ? `${it.progress}%` : it.status}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

// ---- Jobs (full page) ----------------------------------------------
function Jobs({iconStyle}) {
  return (
    <div className="main">
      <div className="page">
        <div className="page-header">
          <div className="g-eyebrow g-eyebrow-accent">Jobs</div>
          <h1>Background work</h1>
          <p>Thumbnailing, imports, bulk tag edits. Cancel anything that's still running. Completed jobs are kept for 1 hour.</p>
        </div>
        <div className="g-card" style={{overflow: 'hidden'}}>
          {JOBS.map(j => <JobRow key={j.id} job={j} iconStyle={iconStyle}/>)}
        </div>
      </div>
    </div>
  );
}

function JobRow({job, iconStyle}) {
  const pct = Math.round((job.done / job.total) * 100);
  return (
    <div className="job-row">
      <div className="job-row-head">
        <span className="name">
          <Icon name={job.kind === 'import' ? 'upload' : job.kind === 'tag' ? 'tag' : 'photo'} size={14} style={iconStyle}/>
          <b>{job.name}</b>
        </span>
        <span className={"status " + job.status}>{job.status}</span>
      </div>
      <div className={"job-progress " + (job.status !== 'running' ? job.status : '')}>
        <div style={{width: pct + '%'}}/>
      </div>
      <div className="job-meta">
        <span>{job.done.toLocaleString()} / {job.total.toLocaleString()}</span>
        <span>{job.error || job.started}</span>
      </div>
    </div>
  );
}

// ---- Jobs drawer (overlay from top bar) ----------------------------
function JobsDrawer({onClose, iconStyle}) {
  return (
    <div className="jobs-drawer">
      <div className="jobs-drawer-head">
        <h3>Jobs</h3>
        <div style={{display: 'flex', gap: 4}}>
          <button className="g-btn g-btn-ghost g-btn-sm g-btn-icon" title="Pause all"><Icon name="pause" size={13} style={iconStyle}/></button>
          <button className="g-btn g-btn-ghost g-btn-sm g-btn-icon" onClick={onClose}><Icon name="close" size={13} style={iconStyle}/></button>
        </div>
      </div>
      <div className="jobs-list">
        {JOBS.map(j => <JobRow key={j.id} job={j} iconStyle={iconStyle}/>)}
      </div>
    </div>
  );
}

// ---- Tags index ----------------------------------------------------
function TagsIndex({iconStyle, onSearchTag}) {
  const [filter, setFilter] = _useSP('');
  const grouped = _useMP(() => {
    const m = new Map();
    for (const {tag, count} of TAG_INDEX) {
      if (filter && !tag.toLowerCase().includes(filter.toLowerCase())) continue;
      const {ns} = parseTag(tag);
      const key = ns || '';
      if (!m.has(key)) m.set(key, []);
      m.get(key).push({tag, count});
    }
    const order = ['', 'rating', 'subject', 'location', 'people', 'color', 'year', 'collection', 'film', 'camera'];
    return order.filter(k => m.has(k)).map(k => ({ns: k, label: k || 'tags', tags: m.get(k)}));
  }, [filter]);

  return (
    <div className="main">
      <div className="page">
        <div className="page-header">
          <div className="g-eyebrow g-eyebrow-accent">Tags</div>
          <h1>{TAG_INDEX.length.toLocaleString()} tags across {FILES.length.toLocaleString()} files</h1>
          <p>Browse by namespace. Click any tag to filter the library.</p>
        </div>

        <div style={{position: 'sticky', top: 0, paddingBottom: 12, background: 'var(--bg)', zIndex: 2}}>
          <input className="g-input" placeholder="Filter tags…" value={filter} onChange={(e) => setFilter(e.target.value)}/>
        </div>

        {grouped.map(g => (
          <section key={g.ns || '__valueless'}>
            <div className="tag-ns-header">
              <h3>{g.label}</h3>
              <span className="count">{g.tags.length.toLocaleString()} {g.tags.length === 1 ? 'tag' : 'tags'}</span>
            </div>
            <div className="tagscloud">
              {g.tags.map(({tag, count}) => {
                const {ns, value} = parseTag(tag);
                return (
                  <div key={tag} className="tagscloud-item" onClick={() => onSearchTag(tag)}>
                    <span style={{overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap'}}>
                      {ns && <span className="ns">{ns}:</span>}<span>{value}</span>
                    </span>
                    <span className="count">{count}</span>
                  </div>
                );
              })}
            </div>
          </section>
        ))}
      </div>
    </div>
  );
}

// ---- Shortcuts -----------------------------------------------------
function Shortcuts({iconStyle}) {
  return (
    <div className="main">
      <div className="page">
        <div className="page-header">
          <div className="g-eyebrow g-eyebrow-accent">Keyboard</div>
          <h1>Shortcuts</h1>
          <p>Press <span className="g-kbd">?</span> from anywhere to open this cheatsheet.</p>
        </div>
        <div className="shortcut-grid">
          {SHORTCUTS.map(g => (
            <div key={g.group} className="shortcut-group">
              <h3>{g.group}</h3>
              {g.items.map((it, i) => (
                <div key={i} className="shortcut-row">
                  <span style={{color: 'var(--text-2)'}}>{it.desc}</span>
                  <span className="shortcut-keys">
                    {it.keys.map((k, j) => <span key={j} className="g-kbd">{k}</span>)}
                  </span>
                </div>
              ))}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

// ---- Settings ------------------------------------------------------
function Settings({iconStyle, user, accentHex, setAccentHex, gridDensity, setGridDensity, onSignOut}) {
  const [token, setToken] = _useSP('•••••••••••••••••••••••');
  const [showToken, setShowToken] = _useSP(false);
  const [thumbSizes, setThumbSizes] = _useSP('256, 512');
  const [maxUpload, setMaxUpload] = _useSP(5);

  const accentPresets = ['#f4d976', '#a3e635', '#fb7185', '#7dd3fc', '#e5e7eb'];
  const accentValue = accentHex || (typeof DEFAULT_ACCENT_HEX === 'string' ? DEFAULT_ACCENT_HEX : '#f4d976');
  const density = typeof gridDensity === 'number' ? gridDensity : 180;

  return (
    <div className="main">
      <div className="page">
        <div className="page-header">
          <div className="g-eyebrow g-eyebrow-accent">Settings</div>
          <h1>Server &amp; library settings</h1>
          <p>These mirror your <code style={{color: 'var(--text-2)'}}>gooru.yaml</code> at <code style={{color: 'var(--text-2)'}}>~/.config/gooru/gooru.yaml</code>. Changes save instantly; the server hot-reloads.</p>
        </div>

        <SettingsSection title="Account" subtitle="Your sign-in identity for this server.">
          <div className="field-row">
            <label>Signed in as</label>
            <div className="field-control" style={{display: 'flex', alignItems: 'center', gap: 8}}>
              <span style={{
                width: 28, height: 28, borderRadius: '50%',
                background: 'var(--accent)', color: 'var(--accent-ink)',
                display: 'grid', placeItems: 'center',
                fontFamily: 'var(--font-display)', fontSize: 14,
              }}>{(user || 'g')[0].toUpperCase()}</span>
              <span style={{fontFamily: 'var(--font-mono)', fontSize: 12.5}}>{user || 'guest'}</span>
              <button className="g-btn g-btn-sm" style={{marginLeft: 'auto'}} onClick={() => onSignOut && onSignOut()}><Icon name="logout" size={13} style={iconStyle}/> Sign out</button>
            </div>
          </div>
          <div className="field-row">
            <label>Password</label>
            <div className="field-control" style={{display: 'flex', gap: 8}}>
              <input className="g-input" type="password" value="••••••••••" readOnly style={{maxWidth: 280}}/>
              <button className="g-btn">Change…</button>
            </div>
          </div>
        </SettingsSection>

        <SettingsSection title="Appearance" subtitle="How the UI looks for you. Stored locally — only affects this browser.">
          <div className="field-row">
            <label>Accent color</label>
            <div className="field-control" style={{display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap'}}>
              <label style={{position: 'relative', width: 32, height: 32, borderRadius: 8, background: accentValue, border: '1px solid var(--border)', cursor: 'pointer', boxShadow: '0 1px 2px rgba(0,0,0,0.06)'}}>
                <input
                  type="color"
                  value={accentValue}
                  onChange={(e) => setAccentHex && setAccentHex(e.target.value)}
                  style={{position: 'absolute', inset: 0, width: '100%', height: '100%', opacity: 0, cursor: 'pointer'}}
                />
              </label>
              <input
                className="g-input"
                value={accentValue}
                onChange={(e) => {
                  const v = e.target.value;
                  if (/^#?[0-9a-fA-F]{0,6}$/.test(v)) {
                    if (setAccentHex) setAccentHex(v.startsWith('#') ? v : '#' + v);
                  }
                }}
                style={{width: 110, fontFamily: 'var(--font-mono)', textTransform: 'uppercase'}}
              />
              <div style={{display: 'flex', gap: 6}}>
                {accentPresets.map((p) => (
                  <button
                    key={p}
                    type="button"
                    title={p}
                    onClick={() => setAccentHex && setAccentHex(p)}
                    aria-label={`Set accent to ${p}`}
                    style={{
                      width: 22, height: 22, padding: 0,
                      borderRadius: '50%',
                      background: p,
                      border: accentValue.toLowerCase() === p.toLowerCase() ? '2px solid var(--text)' : '1px solid var(--border)',
                      cursor: 'pointer',
                    }}
                  />
                ))}
              </div>
              {accentHex && (
                <button
                  type="button"
                  className="g-btn g-btn-ghost g-btn-sm"
                  onClick={() => setAccentHex && setAccentHex(null)}
                  title="Reset to the default sodium yellow"
                >
                  Reset
                </button>
              )}
            </div>
          </div>
          <div className="field-row">
            <label>Grid density</label>
            <div className="field-control" style={{display: 'flex', alignItems: 'center', gap: 12}}>
              <input
                type="range"
                min={50}
                max={500}
                step={1}
                value={density}
                onChange={(e) => setGridDensity && setGridDensity(+e.target.value)}
                style={{width: 240, accentColor: 'var(--accent)'}}
              />
              <span className="g-mono" style={{minWidth: 64, fontFamily: 'var(--font-mono)', fontSize: 12}}>{density} px</span>
              {density !== 180 && (
                <button
                  type="button"
                  className="g-btn g-btn-ghost g-btn-sm"
                  onClick={() => setGridDensity && setGridDensity(180)}
                >
                  Reset
                </button>
              )}
            </div>
            <div className="field-help">Controls the per-cell width of the Library grid. Smaller values pack more files per row.</div>
          </div>
        </SettingsSection>

        <SettingsSection title="Server" subtitle="Listen address & access. Loopback bindings are unauthenticated-safe; network bindings require auth.">
          <div className="field-row">
            <label>Listen address</label>
            <input className="g-input" defaultValue="127.0.0.1:5678" style={{maxWidth: 260, fontFamily: 'var(--font-mono)'}}/>
          </div>
          <div className="field-row">
            <label>Public URL</label>
            <input className="g-input" defaultValue="https://gooru.local" style={{maxWidth: 340, fontFamily: 'var(--font-mono)'}}/>
          </div>
          <div className="field-row">
            <label>CORS origins</label>
            <input className="g-input" placeholder="https://my-other-app.example" style={{fontFamily: 'var(--font-mono)'}}/>
          </div>
        </SettingsSection>

        <SettingsSection title="Auth tokens" subtitle="Bearer tokens for headless clients (gooru CLI, scripts, mobile). Web UI uses your username & password.">
          <div className="field-row">
            <label>Personal token</label>
            <div className="field-control" style={{display: 'flex', gap: 8}}>
              <input className="g-input" value={showToken ? 'sk_live_4f7a91d3c2b86e0a' : token} readOnly style={{maxWidth: 320, fontFamily: 'var(--font-mono)'}}/>
              <button className="g-btn g-btn-icon" onClick={() => setShowToken(!showToken)} title={showToken ? 'Hide' : 'Show'}>
                <Icon name="eye" size={14} style={iconStyle}/>
              </button>
              <button className="g-btn">Rotate</button>
            </div>
          </div>
          <div className="field-help">Last used: 2 min ago · 38 calls in the last hour</div>
        </SettingsSection>

        <SettingsSection title="Library" subtitle="Where files live and how they're indexed.">
          <div className="field-row">
            <label>Upload roots</label>
            <div className="field-control" style={{display: 'flex', flexDirection: 'column', gap: 6}}>
              {[
                {name: 'default', path: '/Users/me/library'},
                {name: 'archive', path: '/Volumes/work/archive'},
              ].map(r => (
                <div key={r.name} style={{display: 'flex', gap: 8, alignItems: 'center', padding: '6px 10px', background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 6, fontFamily: 'var(--font-mono)', fontSize: 12}}>
                  <Icon name="folder" size={14} style={iconStyle} color="var(--text-3)"/>
                  <span style={{color: 'var(--accent)'}}>{r.name}</span>
                  <span style={{color: 'var(--text-3)'}}>·</span>
                  <span>{r.path}</span>
                  <button className="g-btn g-btn-ghost g-btn-sm g-btn-icon" style={{marginLeft: 'auto'}}><Icon name="close" size={11} style={iconStyle}/></button>
                </div>
              ))}
              <button className="g-btn g-btn-sm" style={{alignSelf: 'flex-start'}}><Icon name="plus" size={12} style={iconStyle}/> Add directory</button>
            </div>
          </div>
          <div className="field-row">
            <label>Max upload</label>
            <div className="field-control" style={{display: 'flex', gap: 10, alignItems: 'center'}}>
              <input type="range" min={1} max={20} value={maxUpload} onChange={(e) => setMaxUpload(+e.target.value)} style={{width: 200, accentColor: 'var(--accent)'}}/>
              <span className="g-mono" style={{minWidth: 60}}>{maxUpload} GB</span>
            </div>
          </div>
        </SettingsSection>

        <SettingsSection title="Media processing" subtitle="Thumbnailing & preview generation backends.">
          <div className="field-row">
            <label>libvips</label>
            <div className="field-control" style={{display: 'flex', alignItems: 'center', gap: 8, fontFamily: 'var(--font-mono)', fontSize: 12}}>
              <span style={{width: 7, height: 7, borderRadius: '50%', background: 'var(--ok)'}}/>
              <span>detected · 8.15.1</span>
            </div>
          </div>
          <div className="field-row">
            <label>ffmpeg</label>
            <div className="field-control" style={{display: 'flex', alignItems: 'center', gap: 8, fontFamily: 'var(--font-mono)', fontSize: 12}}>
              <span style={{width: 7, height: 7, borderRadius: '50%', background: 'var(--ok)'}}/>
              <span>detected · 6.1 · /opt/homebrew/bin/ffmpeg</span>
            </div>
          </div>
          <div className="field-row">
            <label>Thumbnail sizes</label>
            <input className="g-input" value={thumbSizes} onChange={(e) => setThumbSizes(e.target.value)} style={{maxWidth: 240, fontFamily: 'var(--font-mono)'}}/>
          </div>
          <div className="field-row">
            <label>Format</label>
            <Segmented value="jpeg" onChange={() => {}} options={[
              {value: 'jpeg', label: 'JPEG'},
              {value: 'webp', label: 'WebP'},
              {value: 'avif', label: 'AVIF'},
            ]}/>
          </div>
        </SettingsSection>
      </div>
    </div>
  );
}

function SettingsSection({title, subtitle, children}) {
  return (
    <section className="settings-section">
      <div className="settings-section-head">
        <h2>{title}</h2>
        <p>{subtitle}</p>
      </div>
      <div className="settings-section-body">{children}</div>
    </section>
  );
}

Object.assign(window, {Upload, Jobs, JobsDrawer, TagsIndex, Shortcuts, Settings});
