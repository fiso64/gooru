from pathlib import Path


def replace(path, old, new):
    p = Path(path)
    text = p.read_text()
    if old not in text:
        raise SystemExit(f'missing pattern in {path}: {old[:80]!r}')
    p.write_text(text.replace(old, new, 1))

# Config: protected URL tokens default on, but only activate when encryption is enabled.
replace('internal/serve/config.go',
'''type EncryptionConfig struct {
\tEnabled bool   `yaml:"enabled"`
\tKeyFile string `yaml:"key_file"`
\tKey     []byte `yaml:"-"`
}''',
'''type EncryptionConfig struct {
\tEnabled        bool   `yaml:"enabled"`
\tKeyFile        string `yaml:"key_file"`
\tOpaqueURLState bool   `yaml:"opaque_url_state"`
\tKey            []byte `yaml:"-"`
}''')
replace('internal/serve/config.go',
'''\t\tDatabase: DatabaseConfig{Path: dbPath},
\t\tAuth: AuthConfig{''',
'''\t\tDatabase:   DatabaseConfig{Path: dbPath},
\t\tEncryption: EncryptionConfig{OpaqueURLState: true},
\t\tAuth: AuthConfig{''')

# UI config tells the bundled client which privacy behaviors are active.
replace('internal/serve/ui_config.go',
'''\tProtectedMode            bool   `json:"protected_mode"`
}''',
'''\tProtectedMode            bool   `json:"protected_mode"`
\tOpaqueURLState           bool   `json:"opaque_url_state"`
}''')
replace('internal/serve/ui_config.go',
'''\t\tProtectedMode:            s.cfg.Encryption.Enabled,
\t})''',
'''\t\tProtectedMode:            s.cfg.Encryption.Enabled,
\t\tOpaqueURLState:           s.cfg.Encryption.Enabled && s.cfg.Encryption.OpaqueURLState,
\t})''')

# Server owns the sealing codec and authenticated API surface.
replace('internal/serve/server.go',
'''\tauth    *AuthStore
}''',
'''\tauth     *AuthStore
\turlState *urlStateCodec
}''')
replace('internal/serve/server.go',
'''\t\tmedia:   NewMediaService(cfg),
\t\tmeta:    metadata,
\t}''',
'''\t\tmedia:    NewMediaService(cfg),
\t\tmeta:     metadata,
\t\turlState: newURLStateCodec(cfg),
\t}''')
replace('internal/serve/server.go',
'''\tmux.Handle("/api/v1/upload-targets", authMiddleware(s.cfg, s.auth, methodHandler(http.MethodGet, s.handleUploadTargets)))''',
'''\tmux.Handle("/api/v1/ui-state/", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleUIState))))
\tmux.Handle("/api/v1/ui-state", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleUIState))))
\tmux.Handle("/api/v1/upload-targets", authMiddleware(s.cfg, s.auth, methodHandler(http.MethodGet, s.handleUploadTargets)))''')

Path('internal/serve/url_state.go').write_text(r'''package serve

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/hmac"
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "encoding/json"
    "errors"
    "io"
    "net/http"
    "strings"
    "time"
)

const opaqueURLStateTTL = 24 * time.Hour

var (
    errInvalidURLStateToken = errors.New("invalid URL state token")
    errExpiredURLStateToken = errors.New("expired URL state token")
)

type browserURLState struct {
    Query  string `json:"query,omitempty"`
    Kind   string `json:"kind,omitempty"`
    Sort   string `json:"sort,omitempty"`
    Order  string `json:"order,omitempty"`
    FileID string `json:"file_id,omitempty"`
}

type sealedBrowserURLState struct {
    State     browserURLState `json:"state"`
    UserID    string          `json:"user_id,omitempty"`
    ExpiresAt int64           `json:"expires_at"`
}

type urlStateCodec struct {
    aead cipher.AEAD
    now  func() time.Time
}

func newURLStateCodec(cfg Config) *urlStateCodec {
    if !cfg.Encryption.Enabled || !cfg.Encryption.OpaqueURLState || len(cfg.Encryption.Key) == 0 {
        return nil
    }
    mac := hmac.New(sha256.New, cfg.Encryption.Key)
    _, _ = mac.Write([]byte("gooru/protected-url-state/v1"))
    key := mac.Sum(nil)
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil
    }
    aead, err := cipher.NewGCM(block)
    if err != nil {
        return nil
    }
    return &urlStateCodec{aead: aead, now: func() time.Time { return time.Now().UTC() }}
}

func (c *urlStateCodec) seal(state browserURLState, userID string) (string, error) {
    if c == nil {
        return "", errInvalidURLStateToken
    }
    payload, err := json.Marshal(sealedBrowserURLState{State: state, UserID: userID, ExpiresAt: c.now().Add(opaqueURLStateTTL).Unix()})
    if err != nil {
        return "", err
    }
    nonce := make([]byte, c.aead.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return "", err
    }
    ciphertext := c.aead.Seal(nil, nonce, payload, []byte("gooru/protected-url-state/v1"))
    token := append(nonce, ciphertext...)
    return base64.RawURLEncoding.EncodeToString(token), nil
}

func (c *urlStateCodec) open(token, userID string) (browserURLState, error) {
    if c == nil {
        return browserURLState{}, errInvalidURLStateToken
    }
    raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
    if err != nil || len(raw) <= c.aead.NonceSize() {
        return browserURLState{}, errInvalidURLStateToken
    }
    plaintext, err := c.aead.Open(nil, raw[:c.aead.NonceSize()], raw[c.aead.NonceSize():], []byte("gooru/protected-url-state/v1"))
    if err != nil {
        return browserURLState{}, errInvalidURLStateToken
    }
    var payload sealedBrowserURLState
    if err := json.Unmarshal(plaintext, &payload); err != nil {
        return browserURLState{}, errInvalidURLStateToken
    }
    if payload.ExpiresAt <= c.now().Unix() {
        return browserURLState{}, errExpiredURLStateToken
    }
    if payload.UserID != userID {
        return browserURLState{}, errInvalidURLStateToken
    }
    return payload.State, nil
}

func requestURLStateUserID(r *http.Request) string {
    if auth, ok := currentAuth(r.Context()); ok {
        return auth.User.ID
    }
    return ""
}

func (s *Server) handleUIState(w http.ResponseWriter, r *http.Request) {
    if s.urlState == nil {
        writeError(w, http.StatusNotFound, "not_found", "protected URL state is not enabled", nil)
        return
    }
    switch r.Method {
    case http.MethodPost:
        if r.URL.Path != "/api/v1/ui-state" {
            writeError(w, http.StatusNotFound, "not_found", "route not found", nil)
            return
        }
        var state browserURLState
        if err := json.NewDecoder(r.Body).Decode(&state); err != nil {
            writeError(w, http.StatusBadRequest, "invalid_request", "invalid URL state", nil)
            return
        }
        token, err := s.urlState.seal(state, requestURLStateUserID(r))
        if err != nil {
            writeError(w, http.StatusInternalServerError, "url_state_failed", "failed to protect URL state", nil)
            return
        }
        writeJSON(w, http.StatusCreated, map[string]string{"token": token})
    case http.MethodGet:
        token := strings.TrimPrefix(r.URL.Path, "/api/v1/ui-state/")
        if token == "" || strings.Contains(token, "/") {
            writeError(w, http.StatusNotFound, "not_found", "URL state not found", nil)
            return
        }
        state, err := s.urlState.open(token, requestURLStateUserID(r))
        if errors.Is(err, errExpiredURLStateToken) {
            writeError(w, http.StatusGone, "url_state_expired", "protected URL state has expired", nil)
            return
        }
        if err != nil {
            writeError(w, http.StatusNotFound, "url_state_invalid", "protected URL state is invalid", nil)
            return
        }
        writeJSON(w, http.StatusOK, state)
    default:
        w.Header().Set("Allow", "GET, POST")
        writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
    }
}
''')

Path('internal/serve/url_state_test.go').write_text(r'''package serve

import (
    "bytes"
    "encoding/base64"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "strings"
    "testing"
    "time"
)

func protectedURLConfig(t *testing.T) Config {
    t.Helper()
    cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
    cfg.Auth.Enabled = false
    cfg.Encryption.Enabled = true
    cfg.Encryption.OpaqueURLState = true
    cfg.Encryption.Key = bytes.Repeat([]byte{0x42}, 32)
    return cfg
}

func TestOpaqueURLStateRoundTripDoesNotExposePlaintext(t *testing.T) {
    codec := newURLStateCodec(protectedURLConfig(t))
    state := browserURLState{Query: "secret:girlfriend", Sort: "name", Order: "asc", FileID: "private-file"}
    token, err := codec.seal(state, "user-a")
    if err != nil { t.Fatal(err) }
    if strings.Contains(token, "secret") || strings.Contains(token, "private") { t.Fatalf("token exposed semantic state: %q", token) }
    got, err := codec.open(token, "user-a")
    if err != nil { t.Fatal(err) }
    if got != state { t.Fatalf("round trip mismatch: %#v != %#v", got, state) }
    if _, err := codec.open(token, "user-b"); !errors.Is(err, errInvalidURLStateToken) { t.Fatalf("expected user binding failure, got %v", err) }
}

func TestOpaqueURLStateExpires(t *testing.T) {
    codec := newURLStateCodec(protectedURLConfig(t))
    now := time.Unix(1000, 0).UTC()
    codec.now = func() time.Time { return now }
    token, err := codec.seal(browserURLState{Query: "secret"}, "")
    if err != nil { t.Fatal(err) }
    codec.now = func() time.Time { return now.Add(opaqueURLStateTTL + time.Second) }
    if _, err := codec.open(token, ""); !errors.Is(err, errExpiredURLStateToken) { t.Fatalf("expected expiry, got %v", err) }
}

func TestOpaqueURLStateHTTPUsesOnlyOpaqueTokenInURL(t *testing.T) {
    server := NewServerWithLibrary(protectedURLConfig(t), emptyLibrary{})
    body := []byte(`{"query":"secret tag","sort":"added","order":"desc","file_id":"private-file"}`)
    createReq := httptest.NewRequest(http.MethodPost, "/api/v1/ui-state", bytes.NewReader(body))
    createRec := httptest.NewRecorder()
    server.Handler().ServeHTTP(createRec, createReq)
    if createRec.Code != http.StatusCreated { t.Fatalf("create expected 201, got %d: %s", createRec.Code, createRec.Body.String()) }
    var created map[string]string
    if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil { t.Fatal(err) }
    token := created["token"]
    if token == "" || strings.Contains(token, "secret") || strings.Contains(token, "private") { t.Fatalf("unexpected token %q", token) }
    if _, err := base64.RawURLEncoding.DecodeString(token); err != nil { t.Fatalf("token not URL safe: %v", err) }

    resolveReq := httptest.NewRequest(http.MethodGet, "/api/v1/ui-state/"+token, nil)
    resolveRec := httptest.NewRecorder()
    server.Handler().ServeHTTP(resolveRec, resolveReq)
    if resolveRec.Code != http.StatusOK { t.Fatalf("resolve expected 200, got %d: %s", resolveRec.Code, resolveRec.Body.String()) }
    if strings.Contains(resolveReq.URL.String(), "secret") || strings.Contains(resolveReq.URL.String(), "private") { t.Fatalf("resolve URL leaked state: %q", resolveReq.URL.String()) }
}
''').write_text(Path('internal/serve/url_state_test.go').read_text().replace('"encoding/json"', '"encoding/json"\n    "errors"'))

# Frontend privacy switches.
Path('frontend/src/lib/api/privacy.ts').write_text('''let protectedReadTransport = false;\nlet opaqueURLState = false;\n\nexport function setProtectedReadTransport(enabled: boolean) { protectedReadTransport = enabled; }\nexport function useProtectedReadTransport() { return protectedReadTransport; }\nexport function setOpaqueURLState(enabled: boolean) { opaqueURLState = enabled; }\nexport function useOpaqueURLState() { return opaqueURLState; }\n''')
replace('frontend/src/lib/components/AppPage.svelte',
'''  import { setProtectedReadTransport } from '$lib/api/privacy';''',
'''  import { setOpaqueURLState, setProtectedReadTransport } from '$lib/api/privacy';''')
replace('frontend/src/lib/components/AppPage.svelte',
'''    protected_mode?: boolean;
  };''',
'''    protected_mode?: boolean;
    opaque_url_state?: boolean;
  };''')
replace('frontend/src/lib/components/AppPage.svelte',
'''    setProtectedReadTransport(config.protected_mode ?? false);''',
'''    setProtectedReadTransport(config.protected_mode ?? false);
    setOpaqueURLState(config.opaque_url_state ?? false);''')

# Raw typed helpers avoid coupling the history feature to generated-client mutation semantics.
replace('frontend/src/lib/api/client.ts',
'''import type { paths } from './openapi';''',
'''import type { paths } from './openapi';
import type { LibraryURLState } from '$lib/utils/appRoute';''')
replace('frontend/src/lib/api/client.ts',
'''  async getFile(id: string, signal?: AbortSignal): Promise<FileItem> {''',
'''  async createURLState(state: LibraryURLState, signal?: AbortSignal): Promise<string> {
    const response = await fetch(`${absoluteBaseURL(this.baseURL)}/ui-state`, {
      method: 'POST', credentials: 'same-origin', signal,
      headers: { 'Content-Type': 'application/json', 'X-Gooru-CSRF': this.csrfToken },
      body: JSON.stringify({ query: state.query, kind: state.kind, sort: state.sort, order: state.order, file_id: state.fileID })
    });
    const payload = await parseJSONResponse<{ token?: string }>(response);
    if (!response.ok || !payload?.token) throw apiErrorFromResponse(response, payload);
    return payload.token;
  }

  async resolveURLState(token: string, signal?: AbortSignal): Promise<LibraryURLState> {
    const response = await fetch(`${absoluteBaseURL(this.baseURL)}/ui-state/${encodeURIComponent(token)}`, { credentials: 'same-origin', signal });
    const payload = await parseJSONResponse<{ query?: string; kind?: string; sort?: FileSort; order?: SortOrder; file_id?: string }>(response);
    if (!response.ok || !payload) throw apiErrorFromResponse(response, payload);
    return { query: payload.query ?? '', kind: payload.kind ?? '', sort: payload.sort ?? 'added', order: payload.order ?? 'desc', fileID: payload.file_id ?? '' };
  }

  async getFile(id: string, signal?: AbortSignal): Promise<FileItem> {''')
replace('frontend/src/lib/api/client.ts',
'''function parseXHRPayload(text: string): unknown {''',
'''async function parseJSONResponse<T>(response: Response): Promise<T | undefined> {
  try { return await response.json() as T; } catch { return undefined; }
}

function apiErrorFromResponse(response: Response, payload: unknown): ApiError {
  const errorPayload = payload as ApiErrorResponse | undefined;
  const error = new ApiError(response.status, errorPayload?.error?.code ?? 'http_error', errorPayload?.error?.message ?? `Request failed with HTTP ${response.status}`);
  if (response.status === 401) unauthorizedHandler?.();
  return error;
}

function parseXHRPayload(text: string): unknown {''')

# History policy: protected mode writes/reads only sealed state tokens.
p = Path('frontend/src/lib/state/libraryWorkflow.svelte.ts')
text = p.read_text()
text = text.replace("import { writable } from 'svelte/store';", "import { get, writable } from 'svelte/store';")
text = text.replace("import { ApiClient } from '$lib/api/client';", "import { ApiClient } from '$lib/api/client';\nimport { useOpaqueURLState } from '$lib/api/privacy';\nimport { authState } from '$lib/stores/auth';")
old = '''  const initialLibraryState = browser && initialRoute === 'library'\n    ? libraryURLStateFromSearch(window.location.search)\n    : defaultLibraryURLState;'''
new = '''  const opaqueURLState = browser && useOpaqueURLState();
  const initialOpaqueToken = opaqueURLState && initialRoute === 'library'
    ? (new URLSearchParams(window.location.search).get('state')?.trim() ?? '')
    : '';
  const initialLibraryState = browser && initialRoute === 'library' && !initialOpaqueToken
    ? libraryURLStateFromSearch(window.location.search)
    : defaultLibraryURLState;'''
if old not in text: raise SystemExit('missing initial library state block')
text = text.replace(old, new, 1)
text = text.replace("  let suggestionDebounce: ReturnType<typeof setTimeout> | undefined;", "  let suggestionDebounce: ReturnType<typeof setTimeout> | undefined;\n  let historyGeneration = 0;\n  let restoredStateKey = opaqueURLState && !window.location.search ? stateKey(initialLibraryState) : '';\n  let initialOpaqueRestorePending = Boolean(initialOpaqueToken);")
old_effect = '''  // Keep top-level navigation, durable library controls, and the active preview\n  // in one browser-history policy. Components remain unaware of the History API,\n  // and a preview URL can be restored independently of the currently loaded page.\n  $effect(() => {\n    if (!browser) return;\n    const pathname = pathForAppRoute(route);\n    const search = route === 'library'\n      ? searchForLibraryURLState({ query: submittedQuery, kind: '', sort, order, fileID: pendingPreviewID })\n      : '';\n    const nextURL = `${pathname}${search}`;\n    const currentURL = `${window.location.pathname}${window.location.search}`;\n    if (currentURL !== nextURL) window.history.pushState(null, '', nextURL);\n  });\n\n  $effect(() => {\n    if (!browser) return;\n    const restoreRoute = () => {\n      const nextRoute = appRouteFromPath(window.location.pathname);\n      const nextLibraryState = nextRoute === 'library'\n        ? libraryURLStateFromSearch(window.location.search)\n        : defaultLibraryURLState;\n      route = nextRoute;\n      const restoredQuery = nextLibraryState.kind\n        ? replaceSidebarKind(nextLibraryState.query, `type:${nextLibraryState.kind}`)\n        : nextLibraryState.query;\n      activeKind = '';\n      activeSavedSearch = '';\n      sort = nextLibraryState.sort;\n      order = nextLibraryState.order;\n      pendingPreviewID = nextLibraryState.fileID;\n      searchDraft.set(restoredQuery);\n      suggestionSearch.set(restoredQuery);\n      setSubmittedSearch(restoredQuery);\n      activeFile = null;\n      selection = emptySelection();\n      selectionAnchorID = '';\n    };\n    window.addEventListener('popstate', restoreRoute);\n    return () => window.removeEventListener('popstate', restoreRoute);\n  });'''
new_effect = '''  function stateKey(state: typeof defaultLibraryURLState) { return JSON.stringify(state); }

  function applyRestoredLibraryState(nextLibraryState: typeof defaultLibraryURLState) {
    restoredStateKey = stateKey(nextLibraryState);
    const restoredQuery = nextLibraryState.kind ? replaceSidebarKind(nextLibraryState.query, `type:${nextLibraryState.kind}`) : nextLibraryState.query;
    activeKind = '';
    activeSavedSearch = '';
    sort = nextLibraryState.sort;
    order = nextLibraryState.order;
    pendingPreviewID = nextLibraryState.fileID;
    searchDraft.set(restoredQuery);
    suggestionSearch.set(restoredQuery);
    setSubmittedSearch(restoredQuery);
    activeFile = null;
    selection = emptySelection();
    selectionAnchorID = '';
  }

  async function resolveOpaqueState(token: string, signal?: AbortSignal) {
    return new ApiClient(get(authState).csrfToken).resolveURLState(token, signal);
  }

  $effect(() => {
    if (!browser || !initialOpaqueRestorePending || !initialOpaqueToken) return;
    initialOpaqueRestorePending = false;
    const controller = new AbortController();
    resolveOpaqueState(initialOpaqueToken, controller.signal).then(applyRestoredLibraryState).catch((error) => {
      if (controller.signal.aborted) return;
      console.warn('Unable to restore protected library URL state', error);
      restoredStateKey = stateKey(defaultLibraryURLState);
      window.history.replaceState(null, '', pathForAppRoute('library'));
    });
    return () => controller.abort();
  });

  // Keep top-level navigation, durable library controls, and the active preview
  // in one browser-history policy. Protected mode seals library state server-side
  // before it enters the address bar; ordinary mode retains readable URLs.
  $effect(() => {
    if (!browser || initialOpaqueRestorePending) return;
    const pathname = pathForAppRoute(route);
    if (route !== 'library') {
      const currentURL = `${window.location.pathname}${window.location.search}`;
      if (currentURL !== pathname) window.history.pushState(null, '', pathname);
      return;
    }
    const state = { query: submittedQuery, kind: '', sort, order, fileID: pendingPreviewID };
    const key = stateKey(state);
    if (restoredStateKey === key) { restoredStateKey = ''; return; }
    if (!opaqueURLState) {
      const nextURL = `${pathname}${searchForLibraryURLState(state)}`;
      const currentURL = `${window.location.pathname}${window.location.search}`;
      if (currentURL !== nextURL) window.history.pushState(null, '', nextURL);
      return;
    }
    if (key === stateKey(defaultLibraryURLState)) {
      if (`${window.location.pathname}${window.location.search}` !== pathname) window.history.pushState(null, '', pathname);
      return;
    }
    const generation = ++historyGeneration;
    const replaceLegacy = !new URLSearchParams(window.location.search).has('state') && window.location.search !== '';
    new ApiClient(get(authState).csrfToken).createURLState(state).then((token) => {
      if (generation !== historyGeneration) return;
      const nextURL = `${pathname}?state=${encodeURIComponent(token)}`;
      if (`${window.location.pathname}${window.location.search}` === nextURL) return;
      if (replaceLegacy) window.history.replaceState(null, '', nextURL);
      else window.history.pushState(null, '', nextURL);
    }).catch((error) => console.warn('Unable to protect library URL state', error));
  });

  $effect(() => {
    if (!browser) return;
    const restoreRoute = async () => {
      const nextRoute = appRouteFromPath(window.location.pathname);
      route = nextRoute;
      if (nextRoute !== 'library') {
        applyRestoredLibraryState(defaultLibraryURLState);
        return;
      }
      const token = opaqueURLState ? (new URLSearchParams(window.location.search).get('state')?.trim() ?? '') : '';
      try {
        const nextLibraryState = token ? await resolveOpaqueState(token) : libraryURLStateFromSearch(window.location.search);
        applyRestoredLibraryState(nextLibraryState);
      } catch (error) {
        console.warn('Unable to restore protected library history state', error);
        applyRestoredLibraryState(defaultLibraryURLState);
        window.history.replaceState(null, '', pathForAppRoute('library'));
      }
    };
    window.addEventListener('popstate', restoreRoute);
    return () => window.removeEventListener('popstate', restoreRoute);
  });'''
if old_effect not in text: raise SystemExit('missing history effects')
text = text.replace(old_effect, new_effect, 1)
p.write_text(text)

# Documentation. Keep config reference concise and make lifecycle explicit.
replace('docs/CONFIG.md',
'''| `encryption.key_file` | empty | Path to a regular file containing a base64-encoded 256-bit key. On non-Windows systems the file must not be readable or writable by group or others. Mutually exclusive with the environment key sources below. |''',
'''| `encryption.key_file` | empty | Path to a regular file containing a base64-encoded 256-bit key. On non-Windows systems the file must not be readable or writable by group or others. Mutually exclusive with the environment key sources below. |
| `encryption.opaque_url_state` | `true` | In protected mode, replace WebUI library query/sort/preview URL parameters with authenticated opaque state tokens. Disable only if permanent readable/bookmarkable query URLs are more important than minimizing browser/proxy URL disclosure. |''')
replace('docs/CONFIG.md',
'''When protected mode is enabled, the bundled WebUI sends free-form file searches and search-suggestion context in authenticated POST request bodies instead of URL query strings. The existing GET endpoints remain available for API/CLI compatibility. This reduces plaintext query exposure in browser/proxy URL history, but HTTPS is still required to protect request bodies in transit.''',
'''When protected mode is enabled, the bundled WebUI sends free-form file searches and search-suggestion context in authenticated POST request bodies instead of URL query strings. The existing GET endpoints remain available for API/CLI compatibility. With `encryption.opaque_url_state` enabled (the default), library query/sort/preview state is also sealed into an authenticated `state` token before entering browser history. Tokens contain no readable query/file state, are bound to the authenticated user, expire after 24 hours, survive server restarts while the same encryption key remains configured, and become unreadable after key rotation. Reload and back/forward work while a token remains valid; long-lived bookmarks intentionally do not have the same permanence as readable URLs. No plaintext token mapping is stored in browser storage. HTTPS is still required to protect request bodies and responses in transit.''')

# Add OpenAPI routes/schemas by inserting before the existing /upload-targets path.
p = Path('docs/openapi.yaml')
text = p.read_text()
anchor = '  /upload-targets:\n'
if anchor not in text: raise SystemExit('missing openapi upload-targets anchor')
block = '''  /ui-state:\n    post:\n      summary: Seal WebUI library state into an opaque protected-mode token\n      security:\n        - cookieAuth: []\n          csrfToken: []\n      requestBody:\n        required: true\n        content:\n          application/json:\n            schema:\n              $ref: '#/components/schemas/BrowserURLState'\n      responses:\n        '201':\n          description: Protected state token\n          content:\n            application/json:\n              schema:\n                type: object\n                required: [token]\n                properties:\n                  token:\n                    type: string\n        '404':\n          $ref: '#/components/responses/NotFound'\n  /ui-state/{token}:\n    get:\n      summary: Resolve an opaque protected-mode WebUI state token\n      security:\n        - cookieAuth: []\n      parameters:\n        - name: token\n          in: path\n          required: true\n          schema:\n            type: string\n      responses:\n        '200':\n          description: Restored library state\n          content:\n            application/json:\n              schema:\n                $ref: '#/components/schemas/BrowserURLState'\n        '404':\n          $ref: '#/components/responses/NotFound'\n        '410':\n          description: Protected state token expired\n'''
text = text.replace(anchor, block + anchor, 1)
# Insert schema near schemas section, before FileListResponse if present.
schema_anchor = '    FileListResponse:\n'
schema = '''    BrowserURLState:\n      type: object\n      properties:\n        query:\n          type: string\n        kind:\n          type: string\n        sort:\n          type: string\n          enum: [added, name, modified, size, kind]\n        order:\n          type: string\n          enum: [asc, desc]\n        file_id:\n          type: string\n'''
if schema_anchor not in text: raise SystemExit('missing FileListResponse schema anchor')
text = text.replace(schema_anchor, schema + schema_anchor, 1)
p.write_text(text)
