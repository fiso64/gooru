package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	core "gooru.local/gooru"
	"gooru.local/internal/database"
	"gooru.local/types"
)

func TestSavedSearchReorderGoldenPath(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	if err := core.Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	defer client.Close()

	authDB, err := database.NewStore(dbPath, false)
	if err != nil {
		t.Fatalf("open auth db: %v", err)
	}
	defer authDB.Close()
	authStore := NewAuthStore(authDB.DB, time.Hour)
	if _, err := authStore.CreateAdmin(context.Background(), "saved-search-test", "correct horse"); err != nil {
		t.Fatalf("create test user: %v", err)
	}
	auth, err := authStore.Login(context.Background(), "saved-search-test", "correct horse")
	if err != nil {
		t.Fatalf("login test user: %v", err)
	}

	cfg := DefaultConfig(filepath.Join(dir, "serve.db"))
	server := NewServerWithLibrary(cfg, NewGooruLibrary(client, false))
	server.SetAuthStore(authStore)

	request := func(method, path string, body io.Reader) *http.Request {
		t.Helper()
		req := httptest.NewRequest(method, path, body)
		req.AddCookie(&http.Cookie{Name: cfg.Auth.CookieName, Value: auth.Token})
		if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete {
			req.Header.Set("X-Gooru-CSRF", auth.CSRFToken)
		}
		return req
	}

	create := func(name string) SavedSearchDTO {
		t.Helper()
		body, err := json.Marshal(savedSearchRequest{Name: name, Query: "tag:" + name, Sort: "added", Order: "desc"})
		if err != nil {
			t.Fatalf("marshal create request: %v", err)
		}
		rec := httptest.NewRecorder()
		req := request(http.MethodPost, "/api/v1/saved-searches", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %q status = %d: %s", name, rec.Code, rec.Body.String())
		}
		var item SavedSearchDTO
		if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
			t.Fatalf("decode created search: %v", err)
		}
		return item
	}

	first := create("first")
	second := create("second")
	third := create("third")

	reorderBody, err := json.Marshal(savedSearchReorderRequest{IDs: []string{third.ID, first.ID, second.ID}})
	if err != nil {
		t.Fatalf("marshal reorder request: %v", err)
	}
	reorderRec := httptest.NewRecorder()
	reorderReq := request(http.MethodPut, "/api/v1/saved-searches/reorder", bytes.NewReader(reorderBody))
	reorderReq.Header.Set("Content-Type", "application/json")
	server.Handler().ServeHTTP(reorderRec, reorderReq)
	if reorderRec.Code != http.StatusOK {
		t.Fatalf("reorder status = %d: %s", reorderRec.Code, reorderRec.Body.String())
	}

	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, request(http.MethodGet, "/api/v1/saved-searches", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", listRec.Code, listRec.Body.String())
	}
	var listed SavedSearchesResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode saved searches: %v", err)
	}
	if len(listed.Items) != 3 {
		t.Fatalf("listed %d saved searches, want 3", len(listed.Items))
	}
	got := []string{listed.Items[0].ID, listed.Items[1].ID, listed.Items[2].ID}
	want := []string{third.ID, first.ID, second.ID}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("saved-search order = %v, want %v", got, want)
		}
	}
}
