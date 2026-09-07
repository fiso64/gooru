package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

func TestBrowseNegativeHiddenTagFacetsAtHTTPBoundary(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	if err := core.Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatal(err)
	}
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	hiddenPhoto := writeTestFile(t, dir, "hidden.jpg", "hidden photo")
	privateVideo := writeTestFile(t, dir, "private.mp4", "private video")
	bothComic := writeTestFile(t, dir, "both.cbz", "both comic")
	visibleOther := writeTestFile(t, dir, "visible.txt", "visible other")
	if _, err := client.TagFiles([]string{hiddenPhoto}, []string{"hidden"}, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := client.TagFiles([]string{privateVideo}, []string{"private:yes"}, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := client.TagFiles([]string{bothComic}, []string{"hidden", "private:yes"}, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := client.TagFiles([]string{visibleOther}, []string{"visible"}, nil, false); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig(dbPath)
	cfg.Auth.Enabled = false
	server := NewServerWithLibrary(cfg, NewGooruLibrary(client, false))

	assertBrowse := func(expression string, wantTotal int, wantFacets map[string]int) {
		t.Helper()
		target := "/api/v1/files?limit=60&sort=added&order=desc&include_facets=true&query=" + url.QueryEscape(expression)
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%q: expected 200, got %d: %s", expression, rec.Code, rec.Body.String())
		}
		var response FileListResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.TotalCount != wantTotal || len(response.Files) != wantTotal {
			t.Fatalf("%q: total/files = %d/%d, want %d", expression, response.TotalCount, len(response.Files), wantTotal)
		}
		got := facetCounts(response.Facets.Kind)
		if len(got) != len(wantFacets) {
			t.Fatalf("%q: facets = %#v, want %#v", expression, got, wantFacets)
		}
		for kind, count := range wantFacets {
			if got[kind] != count {
				t.Fatalf("%q: facet %q = %d, want %d (all %#v)", expression, kind, got[kind], count, got)
			}
		}
	}

	assertBrowse(`-"hidden"`, 2, map[string]int{"video": 1, "other": 1})
	assertBrowse(`-"hidden" -"private:yes"`, 1, map[string]int{"other": 1})
}
