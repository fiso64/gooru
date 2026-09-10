package serve

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNonAdminSessionCannotUsePrivilegedRoutes(t *testing.T) {
	server, _, cleanup := newAuthenticatedBrowseServer(t)
	defer cleanup()
	if _, err := server.auth.createUser(context.Background(), "viewer", "correct horse", "viewer"); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	viewer, err := server.auth.Login(context.Background(), "viewer", "correct horse")
	if err != nil {
		t.Fatalf("login viewer: %v", err)
	}
	files, err := server.library.(*GooruLibrary).client.GetFilesInfoByQuery("kind:image", false)
	if err != nil || len(files) == 0 {
		t.Fatalf("load test files: files=%d err=%v", len(files), err)
	}
	fileID := server.publicFileID(files[0])

	cases := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{name: "upload", method: http.MethodPost, target: "/api/v1/uploads", body: "not multipart"},
		{name: "tag mutation", method: http.MethodPost, target: "/api/v1/files/tags", body: `{"query":"kind:image","tags":["reviewed"]}`},
		{name: "delete file", method: http.MethodDelete, target: "/api/v1/files/" + fileID, body: `{"mode":"untrack"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.target, bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			addAuthCookie(req, server.cfg, viewer)
			addCSRF(req, viewer)
			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, req)
			assertAPIError(t, rec, http.StatusForbidden, "forbidden")
		})
	}
}
