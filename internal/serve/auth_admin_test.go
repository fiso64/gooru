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

func TestReconcileAdminRenamesStableIdentityWithoutRehashingUnchangedPassword(t *testing.T) {
	store := newAuthTestStore(t)
	ctx := context.Background()
	user, err := store.CreateAdmin(ctx, "alice", "correct horse")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	auth, err := store.Login(ctx, "alice", "correct horse")
	if err != nil {
		t.Fatalf("login before reconciliation: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO saved_searches (id, user_id, name, query, sort, "order") VALUES (?, ?, ?, ?, ?, ?)`, "search_test", user.ID, "mine", "tag:test", "name", "asc"); err != nil {
		t.Fatalf("insert saved search: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO saved_search_order (user_id, saved_search_id, position) VALUES (?, ?, 0)`, user.ID, "search_test"); err != nil {
		t.Fatalf("insert saved search order: %v", err)
	}

	hashCalls := 0
	verifyCalls := 0
	baseHash := store.hashPassword
	baseVerify := store.verifyPassword
	store.hashPassword = func(password string) (string, error) {
		hashCalls++
		return baseHash(password)
	}
	store.verifyPassword = func(encoded, password string) (bool, error) {
		verifyCalls++
		return baseVerify(encoded, password)
	}

	updated, err := store.ReconcileAdmin(ctx, user.ID, "renamed", "correct horse")
	if err != nil {
		t.Fatalf("reconcile admin: %v", err)
	}
	if updated.ID != user.ID || updated.Username != "renamed" {
		t.Fatalf("unexpected reconciled user: %+v", updated)
	}
	if verifyCalls != 1 || hashCalls != 0 {
		t.Fatalf("unchanged password cost verify=%d hash=%d, want verify=1 hash=0", verifyCalls, hashCalls)
	}
	lookedUp, err := store.LookupSession(ctx, auth.Token)
	if err != nil {
		t.Fatalf("rename should preserve active session: %v", err)
	}
	if lookedUp.User.Username != "renamed" {
		t.Fatalf("session resolved stale username %q", lookedUp.User.Username)
	}
	var owner string
	if err := store.db.QueryRowContext(ctx, `SELECT user_id FROM saved_searches WHERE id = ?`, "search_test").Scan(&owner); err != nil {
		t.Fatalf("read saved search owner: %v", err)
	}
	if owner != user.ID {
		t.Fatalf("saved search ownership changed: got %q want %q", owner, user.ID)
	}
}

func TestReconcileAdminRotatesChangedPasswordAndRevokesSessions(t *testing.T) {
	store := newAuthTestStore(t)
	ctx := context.Background()
	user, err := store.CreateAdmin(ctx, "alice", "correct horse")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	auth, err := store.Login(ctx, "alice", "correct horse")
	if err != nil {
		t.Fatalf("login before reconciliation: %v", err)
	}

	hashCalls := 0
	verifyCalls := 0
	baseHash := store.hashPassword
	baseVerify := store.verifyPassword
	store.hashPassword = func(password string) (string, error) {
		hashCalls++
		return baseHash(password)
	}
	store.verifyPassword = func(encoded, password string) (bool, error) {
		verifyCalls++
		return baseVerify(encoded, password)
	}

	if _, err := store.ReconcileAdmin(ctx, user.ID, "alice", "different horse"); err != nil {
		t.Fatalf("reconcile changed password: %v", err)
	}
	if verifyCalls != 1 || hashCalls != 1 {
		t.Fatalf("changed password cost verify=%d hash=%d, want verify=1 hash=1", verifyCalls, hashCalls)
	}
	if _, err := store.LookupSession(ctx, auth.Token); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("old session should be revoked, got %v", err)
	}
	if _, err := store.Login(ctx, "alice", "different horse"); err != nil {
		t.Fatalf("new password should authenticate: %v", err)
	}
}

func TestReconcileAdminAdoptsExistingUsernameOrCreatesMissingUser(t *testing.T) {
	store := newAuthTestStore(t)
	ctx := context.Background()
	existing, err := store.CreateAdmin(ctx, "alice", "correct horse")
	if err != nil {
		t.Fatalf("create existing admin: %v", err)
	}
	adopted, err := store.ReconcileAdmin(ctx, "", "alice", "correct horse")
	if err != nil {
		t.Fatalf("adopt existing admin: %v", err)
	}
	if adopted.ID != existing.ID {
		t.Fatalf("adopted wrong identity: got %q want %q", adopted.ID, existing.ID)
	}
	created, err := store.ReconcileAdmin(ctx, "", "bob", "different horse")
	if err != nil {
		t.Fatalf("create missing admin: %v", err)
	}
	if created.ID == "" || created.Username != "bob" {
		t.Fatalf("unexpected created admin: %+v", created)
	}
}

func TestReconcileAdminDoesNotReplaceMissingPersistedIdentity(t *testing.T) {
	store := newAuthTestStore(t)
	ctx := context.Background()
	if _, err := store.CreateAdmin(ctx, "alice", "correct horse"); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if _, err := store.ReconcileAdmin(ctx, "usr_missing", "alice", "correct horse"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("missing persisted identity error = %v, want ErrUserNotFound", err)
	}
}

func TestReconcileAdminRejectsUsernameCollision(t *testing.T) {
	store := newAuthTestStore(t)
	ctx := context.Background()
	alice, err := store.CreateAdmin(ctx, "alice", "correct horse")
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	if _, err := store.CreateAdmin(ctx, "bob", "different horse"); err != nil {
		t.Fatalf("create bob: %v", err)
	}
	if _, err := store.ReconcileAdmin(ctx, alice.ID, "bob", "correct horse"); !errors.Is(err, ErrDuplicateUsername) {
		t.Fatalf("rename collision error = %v, want ErrDuplicateUsername", err)
	}
}
