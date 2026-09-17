package serve

import (
	"context"
	"errors"
	"testing"
)

func TestSetPasswordByUsernamePreservesIdentityAndRevokesSessions(t *testing.T) {
	store := newAuthTestStore(t)
	ctx := context.Background()
	user, err := store.CreateAdmin(ctx, "mac", "correct horse")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	auth, err := store.Login(ctx, "mac", "correct horse")
	if err != nil {
		t.Fatalf("login before password rotation: %v", err)
	}

	updated, err := store.SetPasswordByUsername(ctx, "mac", "different horse")
	if err != nil {
		t.Fatalf("set password: %v", err)
	}
	if updated.ID != user.ID {
		t.Fatalf("password rotation changed user identity: got %q want %q", updated.ID, user.ID)
	}
	if _, err := store.LookupSession(ctx, auth.Token); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("old session should be revoked after password rotation, got %v", err)
	}
	if _, err := store.Login(ctx, "mac", "correct horse"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password should be rejected, got %v", err)
	}
	if _, err := store.Login(ctx, "mac", "different horse"); err != nil {
		t.Fatalf("new password should authenticate: %v", err)
	}
}

func TestSetPasswordByUsernameRejectsMissingUser(t *testing.T) {
	store := newAuthTestStore(t)
	if _, err := store.SetPasswordByUsername(context.Background(), "missing", "correct horse"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("missing user error = %v, want ErrUserNotFound", err)
	}
}
