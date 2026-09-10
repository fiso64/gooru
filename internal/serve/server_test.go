package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestHealthIsUnauthenticated(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)

	NewServer(cfg).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected health 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSecurityHeadersAreApplied(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)

	NewServer(cfg).Handler().ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected nosniff header, got %q", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got == "" {
		t.Fatal("expected Referrer-Policy header")
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("expected X-Frame-Options DENY, got %q", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "frame-ancestors 'none'") {
		t.Fatalf("expected conservative CSP, got %q", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); strings.Contains(got, "script-src 'self' 'unsafe-inline'") {
		t.Fatalf("default CSP should not allow inline scripts, got %q", got)
	}
}

func TestAPIMethodErrorsUseJSONEnvelope(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/health", nil)

	NewServer(cfg).Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusMethodNotAllowed, "method_not_allowed")
	if got := rec.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("expected Allow GET, got %q", got)
	}
}

func TestAPIPathErrorsUseJSONEnvelope(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)

	NewServer(cfg).Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusNotFound, "not_found")
}

func TestPaginationTokensRoundTrip(t *testing.T) {
	page, err := ParsePage("25", "")
	if err != nil {
		t.Fatalf("ParsePage: %v", err)
	}
	token := NextPageToken(page.Offset, page.Limit, page.Limit)
	next, err := ParsePage("25", token)
	if err != nil {
		t.Fatalf("ParsePage token: %v", err)
	}
	if next.Offset != 25 || next.Limit != 25 {
		t.Fatalf("unexpected next page: %+v", next)
	}
}

func TestPreferAsync(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/tags", nil)
	req.Header.Set("Prefer", "wait=10, respond-async")
	if !PreferAsync(req) {
		t.Fatal("expected PreferAsync to detect respond-async")
	}
}

func assertAPIError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("expected status %d, got %d: %s", status, rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected JSON content type, got %q", got)
	}
	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid error JSON: %v", err)
	}
	if body.Error.Code != code {
		t.Fatalf("expected error code %q, got %q", code, body.Error.Code)
	}
}
