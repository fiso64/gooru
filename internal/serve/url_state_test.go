package serve

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
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
	state := browserURLState{Query: "secret:girlfriend", Sort: "name", Order: "asc", FileID: "private-file", Page: 7}
	token, err := codec.seal(state, "user-a")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(token, "secret") || strings.Contains(token, "private") {
		t.Fatalf("token exposed semantic state: %q", token)
	}
	got, err := codec.open(token, "user-a")
	if err != nil {
		t.Fatal(err)
	}
	if got != state {
		t.Fatalf("round trip mismatch: %#v != %#v", got, state)
	}
	if _, err := codec.open(token, "user-b"); !errors.Is(err, errInvalidURLStateToken) {
		t.Fatalf("expected user binding failure, got %v", err)
	}
}

func TestOpaqueURLStateExpires(t *testing.T) {
	codec := newURLStateCodec(protectedURLConfig(t))
	now := time.Unix(1000, 0).UTC()
	codec.now = func() time.Time { return now }
	token, err := codec.seal(browserURLState{Query: "secret"}, "")
	if err != nil {
		t.Fatal(err)
	}
	codec.now = func() time.Time { return now.Add(opaqueURLStateTTL + time.Second) }
	if _, err := codec.open(token, ""); !errors.Is(err, errExpiredURLStateToken) {
		t.Fatalf("expected expiry, got %v", err)
	}
}

func TestOpaqueURLStateRejectsOversizedState(t *testing.T) {
	codec := newURLStateCodec(protectedURLConfig(t))
	if _, err := codec.seal(browserURLState{Query: strings.Repeat("x", maxOpaqueURLStateToken)}, ""); !errors.Is(err, errURLStateTooLarge) {
		t.Fatalf("oversized state error = %v, want errURLStateTooLarge", err)
	}
	if _, err := codec.open(strings.Repeat("x", maxOpaqueURLStateToken+1), ""); !errors.Is(err, errInvalidURLStateToken) {
		t.Fatalf("oversized token error = %v, want errInvalidURLStateToken", err)
	}
}

func TestOpaqueURLStateHTTPUsesOnlyOpaqueTokenInURL(t *testing.T) {
	server := NewServerWithLibrary(protectedURLConfig(t), emptyLibrary{})
	body := []byte(`{"query":"secret tag","sort":"added","order":"desc","file_id":"private-file","page":7}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/ui-state", bytes.NewReader(body))
	createRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}
	var created map[string]string
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	token := created["token"]
	if token == "" || strings.Contains(token, "secret") || strings.Contains(token, "private") {
		t.Fatalf("unexpected token %q", token)
	}
	if _, err := base64.RawURLEncoding.DecodeString(token); err != nil {
		t.Fatalf("token not URL safe: %v", err)
	}

	resolveReq := httptest.NewRequest(http.MethodGet, "/api/v1/ui-state/"+token, nil)
	resolveRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusOK {
		t.Fatalf("resolve expected 200, got %d: %s", resolveRec.Code, resolveRec.Body.String())
	}
	if strings.Contains(resolveReq.URL.String(), "secret") || strings.Contains(resolveReq.URL.String(), "private") {
		t.Fatalf("resolve URL leaked state: %q", resolveReq.URL.String())
	}
	var resolved browserURLState
	if err := json.Unmarshal(resolveRec.Body.Bytes(), &resolved); err != nil {
		t.Fatal(err)
	}
	if resolved.Page != 7 {
		t.Fatalf("resolved page = %d, want 7", resolved.Page)
	}
}

func TestOpaqueURLStateHTTPRejectsOversizedRequest(t *testing.T) {
	server := NewServerWithLibrary(protectedURLConfig(t), emptyLibrary{})
	body := []byte(`{"query":"` + strings.Repeat("x", maxOpaqueURLStateRequest) + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ui-state", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized create expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
