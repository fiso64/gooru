import React from 'react';

// Gooru — top-level App component
//
// Final, baked-in look (Tweaks panel removed). Everything below the
// APP_CONFIG block is the live prototype state.
//
//   logo: wordmark   · type: editorial · icon style: line · layout: grid
//   accent: sodium yellow (user-overridable from Settings → Appearance)
//   grid cell: 180px   (user-overridable from Settings → Appearance)
//   spiral: turns 2.65 · angle 320 · stroke 1.7 · startR 1.2 · oScale 1.5

const {useState: _useSA, useEffect: _useEA, useMemo: _useMA, useCallback: _useCA, useRef: _useRA} = React;

// Frozen visual config — not user-tweakable any more. The Settings screen still
// drives the two appearance knobs we kept (accent color, grid cell size).
const APP_CONFIG = {
  logo: 'wordmark',
  typePair: 'editorial',
  iconStyle: 'line',
  layout: 'grid',
  // Per-variant spiral geometry, applied to the wordmark O's and the topbar logo.
  logoParams: {
    turns:  2.65,
    angle:  320,
    stroke: 1.7,
    startR: 1.2,
    endR:   LOGO_PARAM_DEFAULTS.endR,
    eyeGap: LOGO_PARAM_DEFAULTS.eyeGap,
    oScale: 1.5,
  },
};

const DEFAULT_ACCENT_HEX = '#f4d976';   // sodium yellow, the original default
const DEFAULT_DENSITY    = 180;          // px per grid cell

// Seed items so the queue isn't empty on first render. Real drops are prepended.
const INITIAL_UPLOAD_ITEMS = [
  {id: 'mock-1', name: 'IMG_0921.heic',   size: 4_400_000,   progress: 100, status: 'done',     kind: 'image'},
  {id: 'mock-2', name: 'IMG_0922.heic',   size: 4_220_000,   progress: 100, status: 'done',     kind: 'image'},
  {id: 'mock-3', name: 'IMG_0923.heic',   size: 4_510_000,   progress: 72,  status: 'uploading',kind: 'image'},
  {id: 'mock-4', name: 'IMG_0924.heic',   size: 4_120_000,   progress: 41,  status: 'uploading',kind: 'image'},
  {id: 'mock-5', name: 'roll-2.zip',      size: 184_900_000, progress: 18,  status: 'uploading',kind: 'other'},
  {id: 'mock-6', name: 'venice-clip.mov', size: 92_400_000,  progress: 0,   status: 'queued',   kind: 'video'},
  {id: 'mock-7', name: 'thumbs.db',       size: 21_000,      progress: 100, status: 'skipped',  kind: 'other', error: 'Excluded by .gooru-ignore'},
];

// Compute a contrasting ink color for any accent hex — dark text on light
// accents, light text on dark accents.
function inkForAccent(hex) {
  const m = /^#?([0-9a-f]{6})$/i.exec(String(hex || ''));
  if (!m) return 'oklch(0.20 0.05 80)';
  const n = parseInt(m[1], 16);
  const lin = (c) => { c /= 255; return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4); };
  const L = 0.2126 * lin((n >> 16) & 255) + 0.7152 * lin((n >> 8) & 255) + 0.0722 * lin(n & 255);
  return L > 0.55 ? 'oklch(0.20 0.05 80)' : 'oklch(0.96 0.005 80)';
}

// Inline CSS-variable overrides for an arbitrary accent hex. Derives soft/line
// via color-mix() so highlights, focus rings and pills auto-tint.
function accentStyle(hex) {
  if (!hex) return undefined;
  return {
    '--accent': hex,
    '--accent-ink': inkForAccent(hex),
    '--accent-soft': `color-mix(in oklab, ${hex} 14%, transparent)`,
    '--accent-line': `color-mix(in oklab, ${hex} 45%, transparent)`,
  };
}

function GooruApp({embedded = false, startScreen = 'library'}) {
  // Appearance state — wired to Settings → Appearance.
  const [accentHex, setAccentHex] = _useSA(null);              // null ⇒ use default sodium class
  const [gridDensity, setGridDensity] = _useSA(DEFAULT_DENSITY);
  const logoParams = APP_CONFIG.logoParams;

  // mock auth state — start signed in
  const [user, setUser] = _useSA('alice');
  const [route, setRouteRaw] = _useSA(startScreen);
  // search tokens — pills inside the topbar SearchBar
  const [tokens, setTokensRaw] = _useSA([]);
  // sidebar facets — only meaningful while route === 'library'
  const [activeKind, setActiveKind] = _useSA(null);
  const [activeSavedSearch, setActiveSavedSearch] = _useSA(null);
  const [openedFileId, setOpenedFileId] = _useSA(null);
  const [selection, setSelection] = _useSA(new Set());
  const [showJobs, setShowJobs] = _useSA(false);

  // ── Upload queue, drag-to-upload, global drop overlay ───────────────────────
  const [uploadItems, setUploadItems] = _useSA(() => INITIAL_UPLOAD_ITEMS.map(it => ({...it})));
  const [autoUpload, setAutoUpload] = _useSA(false);
  const [dragOver, setDragOver] = _useSA(false);
  const dragCounter = _useRA(0);

  const updateUploadItem = (id, patch) =>
    setUploadItems(prev => prev.map(it => it.id === id ? {...it, ...patch} : it));

  const removeUploadItem = (id) =>
    setUploadItems(prev => {
      const it = prev.find(x => x.id === id);
      if (it && it.url) { try { URL.revokeObjectURL(it.url); } catch (e) {} }
      return prev.filter(x => x.id !== id);
    });

  // Animate one item from its current progress up to 100% 'done'. Used both for
  // auto-upload on drop and for the explicit 'Upload staged' action.
  const simulateProgress = (item) => {
    let p = item.progress || 0;
    const tick = () => {
      p += 6 + Math.random() * 22;
      if (p >= 100) {
        updateUploadItem(item.id, {progress: 100, status: 'done'});
      } else {
        updateUploadItem(item.id, {progress: Math.round(p)});
        setTimeout(tick, 90 + Math.random() * 90);
      }
    };
    setTimeout(tick, 120 + Math.random() * 250);
  };

  const addFiles = (fileList) => {
    if (!fileList || !fileList.length) return;
    const auto = autoUploadRef.current;
    const newItems = [...fileList].map((file) => {
      const t = (file.type || '').toLowerCase();
      const kind = t.startsWith('image/') ? 'image' : t.startsWith('video/') ? 'video' : 'other';
      return {
        id: 'upl-' + Date.now() + '-' + Math.random().toString(36).slice(2, 8),
        name: file.name,
        size: file.size,
        kind,
        url: (kind === 'image' || kind === 'video') ? URL.createObjectURL(file) : null,
        progress: 0,
        status: auto ? 'uploading' : 'staged',
      };
    });
    setUploadItems(prev => [...newItems, ...prev]);
    if (auto) newItems.forEach(simulateProgress);
  };

  // Promote every staged item to 'uploading' and kick off its progress sim.
  const startStagedUploads = () => {
    const staged = uploadItems.filter(it => it.status === 'staged');
    if (!staged.length) return;
    setUploadItems(prev => prev.map(it =>
      it.status === 'staged' ? {...it, status: 'uploading', progress: 0} : it
    ));
    staged.forEach(it => simulateProgress(it));
  };
  const clearStaged = () => {
    setUploadItems(prev => {
      prev.filter(it => it.status === 'staged' && it.url).forEach(it => {
        try { URL.revokeObjectURL(it.url); } catch (e) {}
      });
      return prev.filter(it => it.status !== 'staged');
    });
  };

  // Stash latest addFiles + setRoute on refs so the window listener can stay
  // mounted once for the lifetime of the app.
  const addFilesRef = _useRA(addFiles);
  const setRouteRef = _useRA(null);
  const autoUploadRef = _useRA(autoUpload);
  addFilesRef.current = addFiles;
  autoUploadRef.current = autoUpload;

  _useEA(() => {
    const isFileDrag = (e) => {
      const types = e.dataTransfer && e.dataTransfer.types;
      if (!types) return false;
      for (let i = 0; i < types.length; i++) if (types[i] === 'Files') return true;
      return false;
    };
    const onEnter = (e) => {
      if (!isFileDrag(e)) return;
      dragCounter.current += 1;
      setDragOver(true);
    };
    const onLeave = (e) => {
      if (!isFileDrag(e)) return;
      dragCounter.current = Math.max(0, dragCounter.current - 1);
      if (dragCounter.current === 0) setDragOver(false);
    };
    const onOver = (e) => { if (isFileDrag(e)) e.preventDefault(); };
    const onDrop = (e) => {
      if (!isFileDrag(e)) return;
      e.preventDefault();
      dragCounter.current = 0;
      setDragOver(false);
      const files = e.dataTransfer && e.dataTransfer.files;
      if (files && files.length) {
        addFilesRef.current(files);
        if (setRouteRef.current) setRouteRef.current('upload');
      }
    };
    window.addEventListener('dragenter', onEnter);
    window.addEventListener('dragleave', onLeave);
    window.addEventListener('dragover', onOver);
    window.addEventListener('drop', onDrop);
    return () => {
      window.removeEventListener('dragenter', onEnter);
      window.removeEventListener('dragleave', onLeave);
      window.removeEventListener('dragover', onOver);
      window.removeEventListener('drop', onDrop);
    };
  }, []);

  // Navigating away from /library clears the library-scoped facets.
  const setRoute = (r) => {
    if (r !== 'library') {
      setActiveKind(null);
      setActiveSavedSearch(null);
    }
    setRouteRaw(r);
  };
  setRouteRef.current = setRoute;
  // User edits to the SearchBar clear the "applied saved search" highlight.
  const setTokens = (next) => {
    setActiveSavedSearch(null);
    setTokensRaw(next);
  };

  // live tag store — copies of FILES so add/remove animates
  const [filesState, setFilesState] = _useSA(() => FILES.map(f => ({...f, tags: [...f.tags]})));

  const handleTagAdd = (fileId, tag) => {
    setFilesState(prev => prev.map(f => f.id === fileId && !f.tags.includes(tag) ? {...f, tags: [...f.tags, tag]} : f));
  };
  const handleTagRemove = (fileId, tag) => {
    setFilesState(prev => prev.map(f => f.id === fileId ? {...f, tags: f.tags.filter(x => x !== tag)} : f));
  };

  const openedIndex = openedFileId ? filesState.findIndex(f => f.id === openedFileId) : -1;
  const openFile = (id) => setOpenedFileId(id);
  const closeFile = () => setOpenedFileId(null);
  const prevFile = () => {
    if (openedIndex <= 0) return;
    setOpenedFileId(filesState[openedIndex - 1].id);
  };
  const nextFile = () => {
    if (openedIndex < 0 || openedIndex >= filesState.length - 1) return;
    setOpenedFileId(filesState[openedIndex + 1].id);
  };

  const toggleKind = (v) => {
    // Kind picks always live on the library view; clicking one routes back if needed.
    setRouteRaw('library');
    setActiveKind(prev => prev === v ? null : v);
  };
  // Run a saved search: parse the textual query into tokens.
  const runSavedSearch = (s) => {
    const parts = s.query.split(/\s+/).filter(Boolean);
    setTokensRaw(parts.map(parseToken));
    setActiveSavedSearch(s.name);
    setRouteRaw('library');
  };
  const onSearchTag = (tag) => {
    const tk = parseToken(tag);
    setTokens(prev => prev.some(p => tokenToString(p) === tokenToString(tk)) ? prev : [...prev, tk]);
    setRouteRaw('library');
  };

  // When no custom hex is set we fall back to the default sodium accent class.
  const accentClass = accentHex ? '' : 'gooru-accent-sodium';
  const typeClass = `gooru-type-${APP_CONFIG.typePair}`;
  const rootStyle = {position: 'absolute', inset: 0, overflow: 'hidden', ...(accentStyle(accentHex) || {})};

  // The Login screen is a special, full-bleed route
  if (route === 'login') {
    return (
      <div className={`gooru-root ${accentClass} ${typeClass}`} style={{...rootStyle, overflow: 'auto'}}>
        <Login onLogin={(u) => { setUser(u); setRoute('library'); }} iconStyle={APP_CONFIG.iconStyle} logoVariant={APP_CONFIG.logo} logoParams={logoParams}/>
      </div>
    );
  }

  return (
    <div className={`gooru-root ${accentClass} ${typeClass}`} style={rootStyle}>
      <div className="app-shell">
        <TopBar
          tokens={tokens}
          setTokens={setTokens}
          iconStyle={APP_CONFIG.iconStyle}
          logoVariant={APP_CONFIG.logo}
          logoParams={logoParams}
          onJobs={() => setShowJobs(s => !s)}
          onUser={() => setRoute('settings')}
          jobsActiveCount={JOBS.filter(j => j.status === 'running').length}
          user={user}
        />
        <Sidebar
          route={route}
          setRoute={(r) => { setRoute(r); setOpenedFileId(null); }}
          iconStyle={APP_CONFIG.iconStyle}
          activeKind={activeKind}
          toggleKind={toggleKind}
          activeSavedSearch={activeSavedSearch}
          runSavedSearch={runSavedSearch}
        />

        {route === 'library'   && <Library
          iconStyle={APP_CONFIG.iconStyle}
          density={gridDensity}
          layoutStyle={APP_CONFIG.layout}
          tokens={tokens}
          activeKind={activeKind}
          onOpenFile={openFile}
          selection={selection}
          setSelection={setSelection}
        />}
        {route === 'tags'      && <TagsIndex iconStyle={APP_CONFIG.iconStyle} onSearchTag={onSearchTag}/>}
        {route === 'upload'    && <Upload
          iconStyle={APP_CONFIG.iconStyle}
          items={uploadItems}
          addFiles={addFiles}
          autoUpload={autoUpload}
          setAutoUpload={setAutoUpload}
          startStagedUploads={startStagedUploads}
          clearStaged={clearStaged}
          removeItem={removeUploadItem}
        />}
        {route === 'jobs'      && <Jobs iconStyle={APP_CONFIG.iconStyle}/>}
        {route === 'settings'  && <Settings
          iconStyle={APP_CONFIG.iconStyle}
          user={user}
          accentHex={accentHex}
          setAccentHex={setAccentHex}
          gridDensity={gridDensity}
          setGridDensity={setGridDensity}
          onSignOut={() => setRoute('login')}
        />}
        {route === 'shortcuts' && <Shortcuts iconStyle={APP_CONFIG.iconStyle}/>}

        {showJobs && <JobsDrawer onClose={() => setShowJobs(false)} iconStyle={APP_CONFIG.iconStyle}/>}

        {openedFileId && (
          <Lightbox
            fileId={openedFileId}
            files={filesState}
            onClose={closeFile}
            onPrev={prevFile}
            onNext={nextFile}
            onTagAdd={handleTagAdd}
            onTagRemove={handleTagRemove}
            iconStyle={APP_CONFIG.iconStyle}
          />
        )}
      </div>
      {dragOver && <DropOverlay iconStyle={APP_CONFIG.iconStyle} autoUpload={autoUpload}/>}
    </div>
  );
}

Object.assign(window, {GooruApp, DEFAULT_ACCENT_HEX, DEFAULT_DENSITY});

// ─── Global drop overlay ─────────────────────────────────────────────
// Shown while a file is being dragged into the app from outside. Pointer-events
// none so the underlying drag events keep propagating (dragenter/leave/drop are
// attached to window in GooruApp).
function DropOverlay({iconStyle, autoUpload}) {
  return (
    <div className="drop-overlay" role="presentation" aria-hidden="true">
      <div className="drop-overlay-card">
        <div className="drop-overlay-icon">
          <Icon name="upload" size={36} style={iconStyle === 'solid' ? 'solid' : 'line'} active/>
        </div>
        <h2>{autoUpload ? 'Drop to upload' : 'Drop to stage'}</h2>
        <p>{autoUpload
          ? 'Files will be content-hashed and uploaded immediately'
          : 'Files will be staged for review — confirm to upload'}</p>
      </div>
    </div>
  );
}
