package serve

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestProtectedModeDefaultsAPIResponsesToNoStore(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = make([]byte, 32)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	NewServer(cfg).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected health 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != protectedAPICacheControl {
		t.Fatalf("expected protected API cache policy %q, got %q", protectedAPICacheControl, got)
	}
}

func TestOrdinaryModeDoesNotForceProtectedAPICachePolicy(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	NewServer(cfg).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected health 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "" {
		t.Fatalf("ordinary mode should not inherit protected cache policy, got %q", got)
	}
}

func TestProtectedAPICachePolicyDoesNotApplyToFrontendRoutes(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/library", nil)

	protectedAPICacheMiddleware(true, next).ServeHTTP(rec, req)

	if got := rec.Header().Get("Cache-Control"); got != "" {
		t.Fatalf("frontend route should retain its own cache policy, got %q", got)
	}
}
