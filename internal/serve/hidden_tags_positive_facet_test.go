package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestHiddenTagsSimplePositiveQueryFacetsExcludeHiddenMatches(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()
	inner := server.library.(*GooruLibrary)

	files, err := inner.client.GetAllFilesInfo()
	if err != nil {
		t.Fatal(err)
	}
	var imageA, imageB string
	for _, file := range files {
		switch filepath.Base(file.Path) {
		case "a.jpg":
			imageA = file.Path
		case "b.png":
			imageB = file.Path
		}
	}
	if imageA == "" || imageB == "" {
		t.Fatalf("missing image fixtures: a=%q b=%q", imageA, imageB)
	}
	if _, err := inner.client.TagFiles([]string{imageA, imageB}, []string{"keep"}, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := inner.client.TagFiles([]string{imageB}, []string{"hidden"}, nil, false); err != nil {
		t.Fatal(err)
	}

	cfg := server.cfg
	cfg.UI.HiddenTags = []string{"hidden"}
	server = NewServerWithLibrary(cfg, inner)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, authedRequest(http.MethodGet, "/api/v1/files?query=keep&include_facets=true"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response FileListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.TotalCount != 1 || len(response.Files) != 1 || response.Files[0].Name != "a.jpg" {
		t.Fatalf("visible files mismatch: %+v", response)
	}
	got := facetCounts(response.Facets.Kind)
	if len(got) != 1 || got["photo"] != 1 {
		t.Fatalf("facets=%#v want photo:1", got)
	}
}
