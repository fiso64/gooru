package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestHiddenTagQueryPolicyPreservesBooleanSemantics(t *testing.T) {
	policy := newHiddenTagQueryPolicy([]string{"bad1", "bad2"})
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{name: "library", query: "", want: `-"bad1" -"bad2"`},
		{name: "ordinary filter", query: "kind:image", want: `(kind:image) -"bad1" -"bad2"`},
		{name: "lift requested tag", query: "bad1 tag1", want: `(bad1 tag1) -"bad2"`},
		{name: "group OR before exclusion", query: "tag1 | tag2", want: `(tag1 | tag2) -"bad1" -"bad2"`},
		{name: "negative reference does not lift", query: "-bad1 tag1", want: `(-bad1 tag1) -"bad1" -"bad2"`},
		{name: "positive tag in group lifts", query: "(bad1 | tag1) tag2", want: `((bad1 | tag1) tag2) -"bad2"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := policy.apply(tc.query); got != tc.want {
				t.Fatalf("apply(%q)=%q want %q", tc.query, got, tc.want)
			}
		})
	}
	if got := policy.baselineFor("bad1 tag1"); got != `-"bad2"` {
		t.Fatalf("dynamic baseline=%q want only bad2 excluded", got)
	}
}

func TestConfigNormalizesAndValidatesHiddenTags(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.UI.HiddenTags = []string{" bad1 ", "bad2", "bad1"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate hidden tags: %v", err)
	}
	if got := strings.Join(cfg.UI.HiddenTags, ","); got != "bad1,bad2" {
		t.Fatalf("normalized hidden tags=%q want bad1,bad2", got)
	}

	cfg = DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.UI.HiddenTags = []string{"-bad"}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "ui.hidden_tags") {
		t.Fatalf("expected hidden tag validation error, got %v", err)
	}
}

func TestHiddenTagsApplyAtBrowseBoundaryButRemainDiscoverable(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()
	inner := server.library.(*GooruLibrary)

	files, err := inner.client.GetAllFilesInfo()
	if err != nil {
		t.Fatalf("list fixture files: %v", err)
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
	if _, err := inner.client.TagFiles([]string{imageA}, []string{"bad1"}, nil, false); err != nil {
		t.Fatalf("tag first hidden image: %v", err)
	}
	if _, err := inner.client.TagFiles([]string{imageB}, []string{"bad1", "bad2"}, nil, false); err != nil {
		t.Fatalf("tag doubly hidden image: %v", err)
	}

	cfg := server.cfg
	cfg.UI.HiddenTags = []string{"bad1", "bad2"}
	server = NewServerWithLibrary(cfg, inner)

	requestPage := func(target string) FileListResponse {
		t.Helper()
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, authedRequest(http.MethodGet, target))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status=%d body=%s", target, rec.Code, rec.Body.String())
		}
		var page FileListResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatalf("decode %s: %v", target, err)
		}
		return page
	}

	library := requestPage("/api/v1/files?include_facets=true")
	if len(library.Files) != 1 || library.Files[0].Name != "notes.txt" || library.TotalCount != 1 || library.LibraryCount != 1 {
		t.Fatalf("default hidden library mismatch: %+v", library)
	}

	bad1 := requestPage("/api/v1/files?query=bad1&include_facets=true")
	if len(bad1.Files) != 1 || bad1.Files[0].Name != "a.jpg" || bad1.TotalCount != 1 || bad1.LibraryCount != 2 {
		t.Fatalf("bad1 query should lift only bad1 exclusion: %+v", bad1)
	}

	both := requestPage("/api/v1/files?query=bad1%20bad2&include_facets=true")
	if len(both.Files) != 1 || both.Files[0].Name != "b.png" || both.TotalCount != 1 || both.LibraryCount != 3 {
		t.Fatalf("query requesting both hidden tags should lift both: %+v", both)
	}

	tagsRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(tagsRec, authedRequest(http.MethodGet, "/api/v1/tags?counts=true"))
	if tagsRec.Code != http.StatusOK {
		t.Fatalf("list tags: status=%d body=%s", tagsRec.Code, tagsRec.Body.String())
	}
	var tags TagListResponse
	if err := json.Unmarshal(tagsRec.Body.Bytes(), &tags); err != nil {
		t.Fatalf("decode tags: %v", err)
	}
	seen := map[string]bool{}
	for _, tag := range tags.Tags {
		seen[tag.Name] = true
	}
	if !seen["bad1"] || !seen["bad2"] {
		t.Fatalf("hidden tags disappeared from tag tab data: %+v", seen)
	}
	if tags.LibraryCount != 1 {
		t.Fatalf("tag-tab library count should honor default exclusions, got %d", tags.LibraryCount)
	}

	suggestions, err := server.library.(SearchLibrary).TagSuggestions(context.Background(), "bad", "", 10)
	if err != nil {
		t.Fatalf("hidden tag suggestions: %v", err)
	}
	seen = map[string]bool{}
	for _, tag := range suggestions {
		seen[tag.Name] = true
	}
	if !seen["bad1"] || !seen["bad2"] {
		t.Fatalf("hidden tags disappeared from completions: %+v", seen)
	}
}
