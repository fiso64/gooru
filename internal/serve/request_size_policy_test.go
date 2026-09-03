package serve

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfigDisablesGlobalRequestBodyLimit(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	if cfg.Server.MaxRequestBodyBytes != 0 {
		t.Fatalf("expected global request body limit to default to disabled, got %d", cfg.Server.MaxRequestBodyBytes)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should accept disabled global request body limit: %v", err)
	}
}

func TestConfigRejectsNegativeGlobalRequestBodyLimit(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Server.MaxRequestBodyBytes = -1
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "server.max_request_body_bytes must be zero or greater") {
		t.Fatalf("expected negative global request body limit validation error, got %v", err)
	}
}

func TestDisabledGlobalRequestBodyLimitDoesNotCapBody(t *testing.T) {
	payload := strings.Repeat("x", 2<<20)
	var got int
	handler := requestSizeMiddleware(0, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		got = len(body)
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/unbounded", strings.NewReader(payload))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if got != len(payload) {
		t.Fatalf("expected %d bytes to reach handler, got %d", len(payload), got)
	}
}

func TestMetadataJSONRoutesRejectOversizedBodiesWithDefaultConfig(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	server := NewServer(cfg)
	payload := `{"username":"` + strings.Repeat("x", int(metadataRequestBodyLimit)) + `","password":"x"}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(payload))
	server.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusRequestEntityTooLarge, "request_too_large")
}

func TestConfiguredGlobalRequestBodyLimitRemainsOuterGuard(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Server.MaxRequestBodyBytes = 16
	server := NewServer(cfg)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"secret"}`))
	server.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusRequestEntityTooLarge, "request_too_large")
}
