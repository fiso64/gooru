from pathlib import Path


def replace(path, old, new):
    p = Path(path)
    text = p.read_text()
    if old not in text:
        raise SystemExit(f'missing patch anchor in {path}: {old[:80]!r}')
    p.write_text(text.replace(old, new, 1))

# Backend: expose protected mode to the browser.
replace('internal/serve/ui_config.go',
'''\tThumbnailSizes           []int  `json:"thumbnail_sizes"`\n''',
'''\tThumbnailSizes           []int  `json:"thumbnail_sizes"`\n\tProtectedMode            bool   `json:"protected_mode"`\n''')
replace('internal/serve/ui_config.go',
'''\t\tThumbnailSizes:           s.cfg.Media.ThumbnailSizes,\n''',
'''\t\tThumbnailSizes:           s.cfg.Media.ThumbnailSizes,\n\t\tProtectedMode:            s.cfg.Encryption.Enabled,\n''')

# Backend: read-only POST endpoint shares the exact list/search service with GET.
replace('internal/serve/server.go',
'''\tmux.Handle("/api/v1/files", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleFiles))))\n''',
'''\tmux.Handle("/api/v1/files", s.protected(requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleFiles))))\n\tmux.Handle("/api/v1/files/search", authMiddleware(s.cfg, s.auth, requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleFileSearch))))\n''')
replace('internal/serve/server.go',
'''\tmux.Handle("/api/v1/search/suggestions", authMiddleware(s.cfg, s.auth, methodHandler(http.MethodGet, s.handleSearchSuggestions)))\n''',
'''\tmux.Handle("/api/v1/search/suggestions", authMiddleware(s.cfg, s.auth, requestBodyLimitMiddleware(metadataRequestBodyLimit, http.HandlerFunc(s.handleSearchSuggestions))))\n''')

old = '''func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {\n\tif s.library == nil {\n\t\twriteError(w, http.StatusServiceUnavailable, "service_unavailable", "file library is not configured", nil)\n\t\treturn\n\t}\n\tpage, err := ParsePage(r.URL.Query().Get("limit"), r.URL.Query().Get("page_token"))\n\tif err != nil {\n\t\twriteError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)\n\t\treturn\n\t}\n\tqueryText := r.URL.Query().Get("query")\n\tsort := normalizeFileSort(r.URL.Query().Get("sort"))\n\torder := normalizeSortOrder(r.URL.Query().Get("order"))\n\tpageResult, err := s.listFilesPage(r.Context(), queryText, page, sort, order)\n'''
new = '''type fileSearchRequest struct {\n\tQuery         string `json:"query"`\n\tLimit         int    `json:"limit"`\n\tPageToken     string `json:"page_token"`\n\tSort          string `json:"sort"`\n\tOrder         string `json:"order"`\n\tIncludeFacets bool   `json:"include_facets"`\n}\n\nfunc (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {\n\treq := fileSearchRequest{\n\t\tQuery: r.URL.Query().Get("query"),\n\t\tPageToken: r.URL.Query().Get("page_token"),\n\t\tSort: r.URL.Query().Get("sort"),\n\t\tOrder: r.URL.Query().Get("order"),\n\t\tIncludeFacets: r.URL.Query().Get("include_facets") == "true",\n\t}\n\tif value := r.URL.Query().Get("limit"); value != "" {\n\t\tif parsed, err := strconv.Atoi(value); err == nil { req.Limit = parsed } else { req.Limit = -1 }\n\t}\n\ts.handleListFilesRequest(w, r, req)\n}\n\nfunc (s *Server) handleFileSearch(w http.ResponseWriter, r *http.Request) {\n\tif r.Method != http.MethodPost {\n\t\tw.Header().Set("Allow", "POST")\n\t\twriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)\n\t\treturn\n\t}\n\tvar req fileSearchRequest\n\tdecoder := json.NewDecoder(r.Body)\n\tdecoder.DisallowUnknownFields()\n\tif err := decoder.Decode(&req); err != nil {\n\t\twriteError(w, http.StatusBadRequest, "invalid_request", "invalid JSON request body", nil)\n\t\treturn\n\t}\n\ts.handleListFilesRequest(w, r, req)\n}\n\nfunc (s *Server) handleListFilesRequest(w http.ResponseWriter, r *http.Request, req fileSearchRequest) {\n\tif s.library == nil {\n\t\twriteError(w, http.StatusServiceUnavailable, "service_unavailable", "file library is not configured", nil)\n\t\treturn\n\t}\n\tlimitText := ""\n\tif req.Limit != 0 { limitText = strconv.Itoa(req.Limit) }\n\tpage, err := ParsePage(limitText, req.PageToken)\n\tif err != nil {\n\t\twriteError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)\n\t\treturn\n\t}\n\tqueryText := req.Query\n\tsort := normalizeFileSort(req.Sort)\n\torder := normalizeSortOrder(req.Order)\n\tpageResult, err := s.listFilesPage(r.Context(), queryText, page, sort, order)\n'''
replace('internal/serve/browse.go', old, new)
replace('internal/serve/browse.go',
'''\tincludeAggregates := r.URL.Query().Get("include_facets") == "true"\n''',
'''\tincludeAggregates := req.IncludeFacets\n''')
# strconv already used later in browse.go; no import change necessary.

# Suggestions: GET remains compatible; POST carries arbitrary text in JSON.
old = '''func (s *Server) handleSearchSuggestions(w http.ResponseWriter, r *http.Request) {\n\tsearch, ok := s.library.(SearchLibrary)\n'''
new = '''type suggestionRequest struct {\n\tQuery    string `json:"q"`\n\tExisting string `json:"existing"`\n\tLimit    int    `json:"limit"`\n}\n\nfunc (s *Server) handleSearchSuggestions(w http.ResponseWriter, r *http.Request) {\n\tvar req suggestionRequest\n\tswitch r.Method {\n\tcase http.MethodGet:\n\t\treq.Query = r.URL.Query().Get("q")\n\t\treq.Existing = r.URL.Query().Get("existing")\n\t\treq.Limit, _ = strconvAtoiDefault(r.URL.Query().Get("limit"), 20)\n\tcase http.MethodPost:\n\t\tdecoder := json.NewDecoder(r.Body)\n\t\tdecoder.DisallowUnknownFields()\n\t\tif err := decoder.Decode(&req); err != nil {\n\t\t\twriteError(w, http.StatusBadRequest, "invalid_request", "invalid JSON request body", nil)\n\t\t\treturn\n\t\t}\n\t\tif req.Limit == 0 { req.Limit = 20 }\n\tdefault:\n\t\tw.Header().Set("Allow", "GET, POST")\n\t\twriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)\n\t\treturn\n\t}\n\tsearch, ok := s.library.(SearchLibrary)\n'''
replace('internal/serve/search_saved.go', old, new)
replace('internal/serve/search_saved.go',
'''\tlimit, _ := strconvAtoiDefault(r.URL.Query().Get("limit"), 20)\n\tprefix := strings.TrimSpace(r.URL.Query().Get("q"))\n\titems, err := search.TagSuggestions(r.Context(), prefix, strings.TrimSpace(r.URL.Query().Get("existing")), limit)\n''',
'''\tlimit := req.Limit\n\tif limit <= 0 { limit = 20 }\n\tprefix := strings.TrimSpace(req.Query)\n\titems, err := search.TagSuggestions(r.Context(), prefix, strings.TrimSpace(req.Existing), limit)\n''')

# Frontend transport policy: configured once from ui-config, consumed centrally by API client.
Path('frontend/src/lib/api/privacy.ts').write_text('''let protectedReadTransport = false;\n\nexport function setProtectedReadTransport(enabled: boolean) {\n  protectedReadTransport = enabled;\n}\n\nexport function useProtectedReadTransport() {\n  return protectedReadTransport;\n}\n''')
replace('frontend/src/lib/components/AppPage.svelte',
'''  import { ApiClient } from '$lib/api/client';\n''',
'''  import { ApiClient } from '$lib/api/client';\n  import { setProtectedReadTransport } from '$lib/api/privacy';\n''')
replace('frontend/src/lib/components/AppPage.svelte',
'''    thumbnail_sizes?: number[];\n''',
'''    thumbnail_sizes?: number[];\n    protected_mode?: boolean;\n''')
replace('frontend/src/lib/components/AppPage.svelte',
'''    runtimeConfig.set({\n''',
'''    setProtectedReadTransport(config.protected_mode ?? false);\n    runtimeConfig.set({\n''')
replace('frontend/src/lib/api/client.ts',
'''import type { paths } from './openapi';\n''',
'''import type { paths } from './openapi';\nimport { useProtectedReadTransport } from './privacy';\n''')
replace('frontend/src/lib/api/client.ts',
'''  async listFiles(params: ListFilesParams = {}): Promise<FileListResponse> {\n    return this.unwrap(\n      this.client.GET('/files', {\n''',
'''  async listFiles(params: ListFilesParams = {}): Promise<FileListResponse> {\n    if (useProtectedReadTransport()) {\n      return this.unwrap(\n        this.client.POST('/files/search', {\n          body: {\n            query: params.query || undefined,\n            limit: params.limit,\n            page_token: params.pageToken,\n            sort: params.sort,\n            order: params.order,\n            include_facets: params.includeFacets || undefined\n          },\n          signal: params.signal\n        })\n      );\n    }\n    return this.unwrap(\n      this.client.GET('/files', {\n''')
replace('frontend/src/lib/api/client.ts',
'''  async searchSuggestions(q = '', limit?: number, existing = '', signal?: AbortSignal): Promise<SuggestionsResponse> {\n    return this.unwrap(\n      this.client.GET('/search/suggestions', {\n''',
'''  async searchSuggestions(q = '', limit?: number, existing = '', signal?: AbortSignal): Promise<SuggestionsResponse> {\n    if (useProtectedReadTransport()) {\n      return this.unwrap(\n        this.client.POST('/search/suggestions', { body: { q: q || undefined, limit, existing: existing || undefined }, signal })\n      );\n    }\n    return this.unwrap(\n      this.client.GET('/search/suggestions', {\n''')

# OpenAPI: add protected POST search and dual suggestion transport, then regenerate TS in workflow.
openapi = Path('docs/openapi.yaml')
text = openapi.read_text()
anchor = '  /files/{id}:\n'
insert = '''  /files/search:\n    post:\n      summary: Search tracked files with query state in the request body.\n      description: Read-only alternative to GET /files for clients that must keep free-form library query text out of browser/proxy URL histories. Uses the same search service and pagination semantics as GET /files.\n      security:\n        - sessionAuth: []\n      requestBody:\n        required: true\n        content:\n          application/json:\n            schema:\n              $ref: "#/components/schemas/FileSearchRequest"\n      responses:\n        "200":\n          description: Page of tracked files.\n          content:\n            application/json:\n              schema:\n                $ref: "#/components/schemas/FileListResponse"\n        "400":\n          $ref: "#/components/responses/BadRequest"\n        "401":\n          $ref: "#/components/responses/Unauthorized"\n'''
if anchor not in text: raise SystemExit('openapi files anchor missing')
text = text.replace(anchor, insert + anchor, 1)
# Add POST beside existing suggestions GET by inserting immediately before namespaces.
sugg_anchor = '  /tags/namespaces:\n'
sugg_post = '''  /search/suggestions/private:\n    post:\n      summary: Return search suggestions with free-form query state in the request body.\n      security:\n        - sessionAuth: []\n      requestBody:\n        required: true\n        content:\n          application/json:\n            schema:\n              $ref: "#/components/schemas/SuggestionRequest"\n      responses:\n        "200":\n          description: Search suggestions.\n          content:\n            application/json:\n              schema:\n                $ref: "#/components/schemas/SuggestionsResponse"\n        "400":\n          $ref: "#/components/responses/BadRequest"\n        "401":\n          $ref: "#/components/responses/Unauthorized"\n'''
# Keep actual runtime path /search/suggestions; replace private path declaration after generation below.
if sugg_anchor not in text: raise SystemExit('openapi suggestions anchor missing')
text = text.replace(sugg_anchor, sugg_post + sugg_anchor, 1)
# Move POST under existing /search/suggestions by textual normalization after YAML generation is not available.
text = text.replace('  /search/suggestions/private:\n    post:', '  /search/suggestions:\n    post:', 1) if '  /search/suggestions:\n' not in text else text
# The document already has /search/suggestions; append a distinct operation cannot duplicate YAML key. Instead splice POST before /tags by extracting indentation under existing key later in a simple safer second pass.
if text.count('  /search/suggestions:\n') > 1:
    first = text.find('  /search/suggestions:\n')
    second = text.find('  /search/suggestions:\n', first + 1)
    block_end = text.find('  /tags/namespaces:\n', second)
    post_block = text[second + len('  /search/suggestions:\n'):block_end]
    text = text[:second] + text[block_end:]
    next_path = text.find('\n  /', first + 1)
    text = text[:next_path] + '\n' + post_block + text[next_path:]
# Schemas.
schema_anchor = '    ErrorResponse:\n'
schemas = '''    FileSearchRequest:\n      type: object\n      properties:\n        query:\n          type: string\n        limit:\n          type: integer\n          minimum: 1\n          maximum: 200\n        page_token:\n          type: string\n        sort:\n          type: string\n          enum: [added, name, modified, size, kind]\n        order:\n          type: string\n          enum: [asc, desc]\n        include_facets:\n          type: boolean\n    SuggestionRequest:\n      type: object\n      properties:\n        q:\n          type: string\n        existing:\n          type: string\n        limit:\n          type: integer\n          minimum: 1\n          maximum: 200\n'''
if schema_anchor not in text: raise SystemExit('openapi schema anchor missing')
text = text.replace(schema_anchor, schemas + schema_anchor, 1)
openapi.write_text(text)

# Regression coverage at the HTTP boundary. Existing GET remains, POST must not require query-string state.
test = Path('internal/serve/protected_search_test.go')
test.write_text(r'''package serve

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "strings"
    "testing"
)

func TestProtectedFileSearchPOSTMatchesListSemanticsWithoutQueryString(t *testing.T) {
    cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
    cfg.Auth.Enabled = false
    body := []byte(`{"query":"kind:image","limit":1,"sort":"name","order":"asc","include_facets":true}`)
    req := httptest.NewRequest(http.MethodPost, "/api/v1/files/search", bytes.NewReader(body))
    rec := httptest.NewRecorder()
    server := NewServerWithLibrary(cfg, emptyLibrary{})
    server.Handler().ServeHTTP(rec, req)
    if rec.Code != http.StatusOK { t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String()) }
    if req.URL.RawQuery != "" { t.Fatalf("protected search leaked query into URL: %q", req.URL.RawQuery) }
    var response FileListResponse
    if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil { t.Fatalf("decode response: %v", err) }
}

func TestProtectedSearchRejectsGET(t *testing.T) {
    cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
    cfg.Auth.Enabled = false
    req := httptest.NewRequest(http.MethodGet, "/api/v1/files/search?query=secret", nil)
    rec := httptest.NewRecorder()
    NewServerWithLibrary(cfg, emptyLibrary{}).Handler().ServeHTTP(rec, req)
    if rec.Code != http.StatusMethodNotAllowed { t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String()) }
}

func TestProtectedSuggestionsPOSTUsesBody(t *testing.T) {
    cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
    cfg.Auth.Enabled = false
    req := httptest.NewRequest(http.MethodPost, "/api/v1/search/suggestions", strings.NewReader(`{"q":"secret tag","existing":"kind:image","limit":5}`))
    rec := httptest.NewRecorder()
    NewServerWithLibrary(cfg, emptyLibrary{}).Handler().ServeHTTP(rec, req)
    if rec.Code != http.StatusServiceUnavailable { t.Fatalf("expected search-service 503 after decoding body, got %d: %s", rec.Code, rec.Body.String()) }
    if req.URL.RawQuery != "" { t.Fatalf("protected suggestions leaked query into URL: %q", req.URL.RawQuery) }
}
''')

# Docs for this slice.
docs = Path('docs/CONFIG.md')
d = docs.read_text()
needle = 'Media responses that contain decrypted protected content use no-store cache policy to reduce plaintext traces in client/proxy caches.\n'
extra = needle + '\nWhen protected mode is enabled, the bundled WebUI sends free-form file searches and search-suggestion context in authenticated POST request bodies instead of URL query strings. The existing GET endpoints remain available for API/CLI compatibility. This reduces plaintext query exposure in browser/proxy URL history, but HTTPS is still required to protect request bodies in transit.\n'
if needle not in d: raise SystemExit('config docs encryption anchor missing')
docs.write_text(d.replace(needle, extra, 1))
