package serve

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestChangePasswordRevokesOtherSessions(t *testing.T) {
	store := newAuthTestStore(t)
	ctx := context.Background()
	user, err := store.CreateAdmin(ctx, "mac", "correct horse")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	current, err := store.Login(ctx, "mac", "correct horse")
	if err != nil {
		t.Fatalf("login current session: %v", err)
	}
	other, err := store.Login(ctx, "mac", "correct horse")
	if err != nil {
		t.Fatalf("login other session: %v", err)
	}

	if err := store.ChangePasswordAndRevokeOtherSessions(ctx, user.ID, current.Session.ID, "correct horse", "new correct horse"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	if _, err := store.LookupSession(ctx, current.Token); err != nil {
		t.Fatalf("current session should remain valid: %v", err)
	}
	if _, err := store.LookupSession(ctx, other.Token); err != ErrSessionNotFound {
		t.Fatalf("other session should be revoked, got %v", err)
	}
	if _, err := store.Login(ctx, "mac", "correct horse"); err != ErrInvalidCredentials {
		t.Fatalf("old password should fail, got %v", err)
	}
	if _, err := store.Login(ctx, "mac", "new correct horse"); err != nil {
		t.Fatalf("new password should authenticate: %v", err)
	}
}

func TestChangePasswordEndpointRevokesOtherSessions(t *testing.T) {
	store := newAuthTestStore(t)
	ctx := context.Background()
	if _, err := store.CreateAdmin(ctx, "mac", "correct horse"); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	current, err := store.Login(ctx, "mac", "correct horse")
	if err != nil {
		t.Fatalf("login current session: %v", err)
	}
	other, err := store.Login(ctx, "mac", "correct horse")
	if err != nil {
		t.Fatalf("login other session: %v", err)
	}

	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	server := NewServer(cfg)
	server.SetAuthStore(store)

	change := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", bytes.NewBufferString(`{"current_password":"correct horse","new_password":"new correct horse"}`))
	addAuthCookie(change, cfg, current)
	addCSRF(change, current)
	changeRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(changeRec, change)
	if changeRec.Code != http.StatusOK {
		t.Fatalf("expected password change 200, got %d: %s", changeRec.Code, changeRec.Body.String())
	}

	currentMe := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	addAuthCookie(currentMe, cfg, current)
	currentRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(currentRec, currentMe)
	if currentRec.Code != http.StatusOK {
		t.Fatalf("current session should remain valid, got %d: %s", currentRec.Code, currentRec.Body.String())
	}

	otherMe := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	addAuthCookie(otherMe, cfg, other)
	otherRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(otherRec, otherMe)
	assertAPIError(t, otherRec, http.StatusUnauthorized, "unauthorized")
}
