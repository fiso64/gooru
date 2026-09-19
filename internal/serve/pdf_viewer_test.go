package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gooru.local/types"
)

func TestPDFViewerCountsPagesThroughLogicalSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pdfinfo")
	script := "#!/bin/sh\nIFS= read -r header\n[ \"$header\" = '%PDF-1.4' ] || exit 3\nprintf 'Pages: 2\\n'\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil { t.Fatal(err) }
	pages, err := (&pdfViewerService{infoPath: path}).pageCount(context.Background(), bytes.NewReader(tinyPDFDocument()))
	if err != nil { t.Fatal(err) }
	if pages != 2 { t.Fatalf("pages = %d, want 2", pages) }
}

func TestProtectedPDFViewerHTTPManifestAndPage(t *testing.T) {
	pdf := tinyPDFDocument()
	path := writeNamedMediaFile(t, "secret.pdf", pdf)
	file := types.FileInfo{ID: 191, Path: path, Hash: "pdf-page-test", Size: int64(len(pdf))}
	server := protectedMediaTestServer(t, file)
	encryptMediaFixture(t, path, server.cfg.Encryption.Key)
	dir := t.TempDir()
	info := filepath.Join(dir, "pdfinfo")
	if err := os.WriteFile(info, []byte("#!/bin/sh\nIFS= read -r header\n[ \"$header\" = '%PDF-1.4' ] || exit 3\nprintf 'Pages: 2\\n'\n"), 0700); err != nil { t.Fatal(err) }
	pngData := testPDFPNG(t, 96, 64)
	imagePath := filepath.Join(dir, "page.png")
	if err := os.WriteFile(imagePath, pngData, 0600); err != nil { t.Fatal(err) }
	renderer := filepath.Join(dir, "renderer")
	argsPath := filepath.Join(dir, "args")
	script := "#!/bin/sh\nIFS= read -r header\n[ \"$header\" = '%PDF-1.4' ] || exit 3\nprintf '%s\\n' \"$*\" >> \"" + argsPath + "\"\ncat \"" + imagePath + "\"\n"
	if err := os.WriteFile(renderer, []byte(script), 0700); err != nil { t.Fatal(err) }
	server.media.pdfViewer = &pdfViewerService{renderer: pdfThumbnailer{path: renderer, version: "fake"}, infoPath: info}
	url := "/api/v1/files/" + fallbackPublicFileID(file.ID) + "/pdf"

	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, authedRequest(http.MethodGet, url))
	if res.Code != http.StatusOK { t.Fatalf("manifest: %d %s", res.Code, res.Body.String()) }
	var manifest pdfDocumentManifest
	if err := json.Unmarshal(res.Body.Bytes(), &manifest); err != nil { t.Fatal(err) }
	if manifest.PageCount != 2 || manifest.PageURLPrefix != url+"?page=" { t.Fatalf("manifest = %+v", manifest) }
	if res.Header().Get("Cache-Control") != "private, no-store" { t.Fatal("manifest cached in protected mode") }

	for i, wantCache := range []string{"miss", "hit"} {
		res = httptest.NewRecorder()
		server.Handler().ServeHTTP(res, authedRequest(http.MethodGet, url+"?page=2"))
		if res.Code != http.StatusOK { t.Fatalf("image %d: %d %s", i, res.Code, res.Body.String()) }
		if !bytes.Equal(res.Body.Bytes(), pngData) { t.Fatal("wrong PDF page raster") }
		if got := res.Header().Get("X-Gooru-Cache"); got != wantCache { t.Fatalf("cache %q, want %q", got, wantCache) }
		if got := res.Header().Get("Cache-Control"); got != "private, no-store" { t.Fatalf("unsafe protected response: %q", got) }
	}
	args, err := os.ReadFile(argsPath)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(args), "-f 2 -l 2") { t.Fatalf("renderer selected wrong page: %q", args) }
}
