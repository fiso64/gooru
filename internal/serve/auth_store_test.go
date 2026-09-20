package serve

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestAuthStoreCleansExpiredAndRevokedSessions(t *testing.T) {
	store := newAuthTestStore(t)
	ctx := context.Background()
	if _, err := store.CreateAdmin(ctx, "mac", "correct horse"); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	expired, err := store.Login(ctx, "mac", "correct horse")
	if err != nil {
		t.Fatalf("login expired candidate: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE sessions SET expires_at = ? WHERE id = ?`, time.Now().Add(-time.Hour).UTC(), expired.Session.ID); err != nil {
		t.Fatalf("expire session: %v", err)
	}
	store.nextCleanupAt = time.Time{}
	if _, err := store.Login(ctx, "mac", "correct horse"); err != nil {
		t.Fatalf("login should clean expired sessions: %v", err)
	}
	if sessionExists(t, store, expired.Session.ID) {
		t.Fatal("expected login cleanup to remove expired session")
	}

	revoked, err := store.Login(ctx, "mac", "correct horse")
	if err != nil {
		t.Fatalf("login revoked candidate: %v", err)
	}
	if err := store.RevokeSession(ctx, revoked.Session.ID); err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	if err := store.CleanupExpiredSessions(ctx); err != nil {
		t.Fatalf("cleanup revoked sessions: %v", err)
	}
	if sessionExists(t, store, revoked.Session.ID) {
		t.Fatal("expected revoke cleanup to remove revoked session")
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

func TestAuthEndpointsAreStableWhenAuthDisabled(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	server := NewServer(cfg)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/auth/login"},
		{http.MethodPost, "/api/v1/auth/logout"},
		{http.MethodGet, "/api/v1/auth/me"},
		{http.MethodPost, "/api/v1/auth/change-password"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		assertAPIError(t, rec, http.StatusNotFound, "not_found")
	}
}

func newAuthTestStore(t *testing.T) *AuthStore {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if err := database.RunMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	store := NewAuthStore(db, time.Hour)
	store.hashPassword = fastTestHashPassword
	store.verifyPassword = fastTestVerifyPassword
	return store
}

func fastTestHashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	return "test-hash:" + password, nil
}

func fastTestVerifyPassword(encoded, password string) (bool, error) {
	hash, err := fastTestHashPassword(password)
	if err != nil {
		return false, err
	}
	return encoded == hash, nil
}

func attachTestAuth(t *testing.T, server *Server) AuthSession {
	t.Helper()
	store := newAuthTestStore(t)
	auth := newTestAuthSession(t, store)
	server.SetAuthStore(store)
	return auth
}

func newTestAuthSession(t *testing.T, store *AuthStore) AuthSession {
	t.Helper()
	ctx := context.Background()
	now := store.now()
	user := User{
		ID:        "usr_test",
		Username:  "testadmin",
		Role:      adminRole,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO users (id, username, password_hash, role, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)`, user.ID, user.Username, "unused-test-password-hash", user.Role, user.CreatedAt, user.UpdatedAt); err != nil {
		t.Fatalf("insert test admin: %v", err)
	}
	auth, err := store.createSession(ctx, user)
	if err != nil {
		t.Fatalf("create test session: %v", err)
	}
	return auth
}

func addAuthCookie(req *http.Request, cfg Config, auth AuthSession) {
	req.AddCookie(&http.Cookie{Name: cfg.Auth.CookieName, Value: auth.Token})
}

func addCSRF(req *http.Request, auth AuthSession) {
	req.Header.Set("X-Gooru-CSRF", auth.CSRFToken)
}

func sessionExists(t *testing.T, store *AuthStore, id string) bool {
	t.Helper()
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, id).Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	return count > 0
}

func TestFirstRunConfigAdminLoginUploadsFile(t *testing.T) {
 testDefaultUploadRoot(t)
 cfgFile:=filepath.Join(t.TempDir(),"serve.yaml")
 writeConfig(t,cfgFile,"{}")
 cfg,err:=LoadConfig(cfgFile,filepath.Join(t.TempDir(),"gooru.db"),Overrides{})
 if err!=nil {t.Fatal(err)}
 store:=newAuthTestStore(t)
 if _,err:=store.CreateAdmin(context.Background(),"alice","correct horse");err!=nil {t.Fatal(err)}
 library:=&recordingUploadLibrary{}
 server:=NewServerWithLibrary(cfg,library)
 server.SetAuthStore(store)
 unauthenticated:=httptest.NewRecorder()
 server.Handler().ServeHTTP(unauthenticated,uploadRequest(t,map[string]string{"first.txt":"hello"},nil))
 assertAPIError(t,unauthenticated,http.StatusUnauthorized,"unauthorized")
 login:=httptest.NewRecorder()
 server.Handler().ServeHTTP(login,httptest.NewRequest(http.MethodPost,"/api/v1/auth/login",bytes.NewBufferString(`{"username":"alice","password":"correct horse"}`)))
 if login.Code!=http.StatusOK {t.Fatalf("admin login %d: %s",login.Code,login.Body.String())}
 var session authMeResponse
 if err:=json.Unmarshal(login.Body.Bytes(),&session);err!=nil {t.Fatal(err)}
 cookies:=login.Result().Cookies()
 if session.CSRFToken=="" || len(cookies)!=1 {t.Fatal("login did not establish session and CSRF token")}
 noCSRF:=uploadRequest(t,map[string]string{"first.txt":"hello"},nil)
 noCSRF.AddCookie(cookies[0])
 forbidden:=httptest.NewRecorder()
 server.Handler().ServeHTTP(forbidden,noCSRF)
 assertAPIError(t,forbidden,http.StatusForbidden,"csrf_required")
 request:=uploadRequest(t,map[string]string{"first.txt":"hello"},nil)
 request.AddCookie(cookies[0])
 request.Header.Set("X-Gooru-CSRF",session.CSRFToken)
 response:=httptest.NewRecorder()
 server.Handler().ServeHTTP(response,request)
 if response.Code!=http.StatusOK {t.Fatalf("admin upload %d: %s",response.Code,response.Body.String())}
 content,err:=os.ReadFile(filepath.Join(cfg.Uploads.Targets[0].Path,"first.txt"))
 if err!=nil || string(content)!="hello" {t.Fatalf("uploaded file=%q err=%v",content,err)}
 if len(library.files)!=1 {t.Fatalf("upload was not passed to importer: %+v",library.files)}
}
