package serve

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"gooru.local/internal/database"
)

func TestPasswordHashVerify(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if hash == "correct horse" {
		t.Fatal("password hash stored plaintext")
	}
	ok, err := VerifyPassword(hash, "correct horse")
	if err != nil || !ok {
		t.Fatalf("expected password verification success, ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword(hash, "wrong password")
	if err != nil || ok {
		t.Fatalf("expected password verification failure, ok=%v err=%v", ok, err)
	}
}

func TestAuthStoreUserSessionLifecycle(t *testing.T) {
	store := newAuthTestStore(t)
	ctx := context.Background()
	if _, err := store.CreateAdmin(ctx, "mac", "correct horse"); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if _, err := store.CreateAdmin(ctx, "mac", "correct horse"); err != ErrDuplicateUsername {
		t.Fatalf("expected duplicate username, got %v", err)
	}
	if _, err := store.Login(ctx, "mac", "bad password"); err != ErrInvalidCredentials {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
	auth, err := store.Login(ctx, "mac", "correct horse")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if auth.Token == "" || auth.CSRFToken == "" || auth.User.Role != adminRole {
		t.Fatalf("unexpected auth session: %+v", auth)
	}
	lookedUp, err := store.LookupSession(ctx, auth.Token)
	if err != nil {
		t.Fatalf("lookup session: %v", err)
	}
	if lookedUp.User.ID != auth.User.ID {
		t.Fatalf("session resolved wrong user: %+v", lookedUp.User)
	}
	if !store.VerifyCSRF(auth, auth.CSRFToken) {
		t.Fatal("expected CSRF token to verify")
	}
	if err := store.RevokeSession(ctx, auth.Session.ID); err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	if _, err := store.LookupSession(ctx, auth.Token); err != ErrSessionNotFound {
		t.Fatalf("expected revoked session rejection, got %v", err)
	}
}

func TestAuthStoreRejectsDisabledAndExpiredSessions(t *testing.T) {
	store := newAuthTestStore(t)
	ctx := context.Background()
	user, err := store.CreateAdmin(ctx, "mac", "correct horse")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE users SET disabled_at = ? WHERE id = ?`, time.Now().UTC(), user.ID); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	if _, err := store.Login(ctx, "mac", "correct horse"); err != ErrDisabledUser {
		t.Fatalf("expected disabled user rejection, got %v", err)
	}

	if _, err := store.db.Exec(`UPDATE users SET disabled_at = NULL WHERE id = ?`, user.ID); err != nil {
		t.Fatalf("enable user: %v", err)
	}
	auth, err := store.Login(ctx, "mac", "correct horse")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE sessions SET expires_at = ? WHERE id = ?`, time.Now().Add(-time.Hour).UTC(), auth.Session.ID); err != nil {
		t.Fatalf("expire session: %v", err)
	}
	if _, err := store.LookupSession(ctx, auth.Token); err != ErrSessionNotFound {
		t.Fatalf("expected expired session rejection, got %v", err)
	}
}

func TestAuthEndpointsRequireSessionAndCSRF(t *testing.T) {
	store := newAuthTestStore(t)
	if _, err := store.CreateAdmin(context.Background(), "mac", "correct horse"); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	server := NewServer(cfg)
	server.SetAuthStore(store)

	loginBody := bytes.NewBufferString(`{"username":"mac","password":"correct horse"}`)
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", loginBody)
	loginRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d: %s", loginRec.Code, loginRec.Body.String())
	}
	var login authMeResponse
	if err := json.Unmarshal(loginRec.Body.Bytes(), &login); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if login.CSRFToken == "" || login.User.Username != "mac" {
		t.Fatalf("unexpected login response: %+v", login)
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly {
		t.Fatalf("expected HttpOnly session cookie, got %+v", cookies)
	}

	missingCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	missingCSRF.AddCookie(cookies[0])
	missingCSRFRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingCSRFRec, missingCSRF)
	assertAPIError(t, missingCSRFRec, http.StatusForbidden, "csrf_required")

	logout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logout.AddCookie(cookies[0])
	logout.Header.Set("X-Gooru-CSRF", login.CSRFToken)
	logoutRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(logoutRec, logout)
	if logoutRec.Code != http.StatusOK {
		t.Fatalf("expected logout 200, got %d: %s", logoutRec.Code, logoutRec.Body.String())
	}

	me := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	me.AddCookie(cookies[0])
	meRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(meRec, me)
	assertAPIError(t, meRec, http.StatusUnauthorized, "unauthorized")
}

func newAuthTestStore(t *testing.T) *AuthStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "auth.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.RunMigrations(db, dbPath); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return NewAuthStore(db, time.Hour)
}
