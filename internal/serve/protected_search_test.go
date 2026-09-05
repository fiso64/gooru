package serve

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
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if req.URL.RawQuery != "" {
		t.Fatalf("protected search leaked query into URL: %q", req.URL.RawQuery)
	}
	var response FileListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func TestProtectedSearchRejectsGET(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/search?query=secret", nil)
	rec := httptest.NewRecorder()
	NewServerWithLibrary(cfg, emptyLibrary{}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestProtectedSuggestionsPOSTUsesBody(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	req := httptest.NewRequest(http.MethodPost, "/api/v1/search/suggestions", strings.NewReader(`{"q":"secret tag","existing":"kind:image","limit":5}`))
	rec := httptest.NewRecorder()
	NewServerWithLibrary(cfg, emptyLibrary{}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected search-service 503 after decoding body, got %d: %s", rec.Code, rec.Body.String())
	}
	if req.URL.RawQuery != "" {
		t.Fatalf("protected suggestions leaked query into URL: %q", req.URL.RawQuery)
	}
}
