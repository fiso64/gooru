package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestAuthCSRFRemainsStableAcrossSessionRefreshes(t *testing.T) {
	store := newAuthTestStore(t)
	if _, err := store.CreateAdmin(context.Background(), "mac", "correct horse"); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	server := NewServer(cfg)
	server.SetAuthStore(store)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"mac","password":"correct horse"}`))
	loginRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d: %s", loginRec.Code, loginRec.Body.String())
	}
	var login authMeResponse
	if err := json.Unmarshal(loginRec.Body.Bytes(), &login); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if login.CSRFToken == "" {
		t.Fatal("expected login CSRF token")
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected session cookie, got %d", len(cookies))
	}

	meToken := func() string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.AddCookie(cookies[0])
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected me 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var response authMeResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode me: %v", err)
		}
		return response.CSRFToken
	}

	firstTabToken := meToken()
	secondTabToken := meToken()
	if firstTabToken != login.CSRFToken || secondTabToken != firstTabToken {
		t.Fatalf("expected stable CSRF token, login=%q first=%q second=%q", login.CSRFToken, firstTabToken, secondTabToken)
	}

	// Simulate the first tab mutating after a second tab refreshed /auth/me.
	// A wrong current password should reach the handler and return 401; a stale
	// CSRF token would instead be rejected by middleware with 403.
	changeReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", bytes.NewBufferString(`{"current_password":"wrong","new_password":"new correct horse"}`))
	changeReq.AddCookie(cookies[0])
	changeReq.Header.Set("X-Gooru-CSRF", firstTabToken)
	changeRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(changeRec, changeReq)
	assertAPIError(t, changeRec, http.StatusUnauthorized, "unauthorized")
}

func TestStableCSRFPreservesLegacyTokenDuringUpgrade(t *testing.T) {
	store := newAuthTestStore(t)
	if _, err := store.CreateAdmin(context.Background(), "mac", "correct horse"); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	auth, err := store.Login(context.Background(), "mac", "correct horse")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	legacyHash := auth.Session.CSRFHash

	stable, err := store.StableCSRF(auth)
	if err != nil {
		t.Fatalf("stable csrf: %v", err)
	}
	if stable == "" || stable == auth.CSRFToken {
		t.Fatalf("expected stable derived token distinct from legacy random token")
	}
	if !store.VerifyCSRFToken(auth, stable) {
		t.Fatal("stable token should verify")
	}
	if !store.VerifyCSRFToken(auth, auth.CSRFToken) {
		t.Fatal("legacy token should remain valid during transition")
	}
	var storedHash string
	if err := store.db.QueryRow(`SELECT csrf_token_hash FROM sessions WHERE id = ?`, auth.Session.ID).Scan(&storedHash); err != nil {
		t.Fatalf("read stored csrf hash: %v", err)
	}
	if storedHash != legacyHash {
		t.Fatalf("issuing stable token should not mutate stored legacy hash")
	}

	again, err := store.StableCSRF(auth)
	if err != nil {
		t.Fatalf("stable csrf again: %v", err)
	}
	if again != stable {
		t.Fatalf("expected repeat stable token, first=%q second=%q", stable, again)
	}
}
