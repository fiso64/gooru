package serve

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gooru.local/types"
)

func TestContentRouteBrowserDocumentInspectsTextSafely(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "notes.txt", body: "plain text\n"},
		{name: "README.md", body: "# markdown\n"},
		{name: "data.json", body: "{\"ok\":true}\n"},
		{name: "config.yaml", body: "enabled: true\n"},
		{name: "source.go", body: "package example\n"},
		{name: "active.html", body: "<script>alert(1)</script>"},
		{name: "script.js", body: "alert(1)"},
		{name: "vector.svg", body: "<svg xmlns=\"http://www.w3.org/2000/svg\"><script>alert(1)</script></svg>"},
	}

	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeNamedMediaFile(t, tc.name, []byte(tc.body))
			fileID := int64(100 + index)
			server := newMediaTestServer(t, types.FileInfo{ID: fileID, Path: path, Hash: "document-text", Size: int64(len(tc.body))})
			req := authedRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(fileID)+"/content")
			req.Header.Set("Sec-Fetch-Dest", "document")
			rec := httptest.NewRecorder()

			server.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
				t.Fatalf("expected inert text/plain for %s, got %q", tc.name, got)
			}
			if got := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "inline;") {
				t.Fatalf("expected inline disposition for %s, got %q", tc.name, got)
			}
			if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Fatalf("expected nosniff for %s, got %q", tc.name, got)
			}
			if got := rec.Body.String(); got != tc.body {
				t.Fatalf("unexpected body for %s: %q", tc.name, got)
			}
		})
	}
}

func TestContentRouteBrowserDocumentOpensPDFInline(t *testing.T) {
	body := []byte("%PDF-1.7\n% fake test pdf\n")
	path := writeNamedMediaFile(t, "document.pdf", body)
	server := newMediaTestServer(t, types.FileInfo{ID: 120, Path: path, Hash: "document-pdf", Size: int64(len(body))})
	req := authedRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(120)+"/content")
	req.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("expected application/pdf, got %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "inline;") {
		t.Fatalf("expected inline disposition, got %q", got)
	}
}

func TestDownloadRouteKeepsAttachmentForBrowserDocument(t *testing.T) {
	body := []byte("download me\n")
	path := writeNamedMediaFile(t, "notes.txt", body)
	server := newMediaTestServer(t, types.FileInfo{ID: 121, Path: path, Hash: "document-download", Size: int64(len(body))})
	req := authedRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(121)+"/download")
	req.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "attachment;") {
		t.Fatalf("expected attachment disposition, got %q", got)
	}
}

func TestOriginalDocumentNavigationFallsBackToBrowserAcceptHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/content", nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if !isOriginalDocumentNavigation(req) {
		t.Fatal("expected browser document Accept header to count as document navigation")
	}

	req.Header.Set("Sec-Fetch-Dest", "image")
	if isOriginalDocumentNavigation(req) {
		t.Fatal("explicit non-document Fetch Metadata must override Accept fallback")
	}
}

func TestEmbeddedPDFUsesInlineDocumentPolicyWithoutEnablingActiveContent(t *testing.T) {
	for _, tc := range []struct {
		name, contents, wantType string
	}{
		{name: "document.pdf", contents: "%PDF-1.7\n", wantType: "application/pdf"},
		{name: "active.html", contents: "<script>alert(1)</script>", wantType: "text/plain; charset=utf-8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeNamedMediaFile(t, tc.name, []byte(tc.contents))
			server := newMediaTestServer(t, types.FileInfo{ID: 122, Path: path, Hash: "embedded-document", Size: int64(len(tc.contents))})
			req := authedRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(122)+"/content")
			req.Header.Set("Sec-Fetch-Dest", "iframe")
			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); got != tc.wantType {
				t.Fatalf("Content-Type = %q, want %q", got, tc.wantType)
			}
			if got := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "inline;") {
				t.Fatalf("embedded content disposition = %q", got)
			}
			if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Fatalf("X-Content-Type-Options = %q", got)
			}
		})
	}
}
