package serve

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func requestAround(t *testing.T, server *Server, id, query, sort, order string, count int) (fileAroundResponse, int) {
	t.Helper()
	body, _ := json.Marshal(fileAroundRequest{FileID: id, Query: query, Sort: sort, Order: order, Count: count})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/files/around", bytes.NewReader(body)))
	var result fileAroundResponse
	if recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatalf("decode neighbors: %v", err)
		}
	}
	return result, recorder.Code
}

func TestFileNeighborsMatchFullFilteredListingInBothDirections(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()
	queries := []string{"", "kind:image", "kind:text", "kind:image | kind:text", "kind:image -kind:text"}
	sorts := []string{"added", "name", "size", "modified", "kind"}
	for _, query := range queries {
		for _, sort := range sorts {
			for _, order := range []string{"asc", "desc"} {
				t.Run(fmt.Sprintf("%s/%s/%s", query, sort, order), func(t *testing.T) {
					target := fmt.Sprintf("/api/v1/files?query=%s&sort=%s&order=%s&limit=200", url.QueryEscape(query), sort, order)
					recorder := httptest.NewRecorder()
					server.Handler().ServeHTTP(recorder, authedRequest(http.MethodGet, target))
					if recorder.Code != http.StatusOK {
						t.Fatalf("listing: %d %s", recorder.Code, recorder.Body.String())
					}
					var listing FileListResponse
					if err := json.Unmarshal(recorder.Body.Bytes(), &listing); err != nil {
						t.Fatal(err)
					}
					for index, file := range listing.Files {
						neighbors, status := requestAround(t, server, file.ID, query, sort, order, 5)
						if status != http.StatusOK {
							t.Fatalf("around %s: HTTP %d", file.ID, status)
						}
						wantLength := len(listing.Files) - 1
						if wantLength > 5 {
							wantLength = 5
						}
						if len(neighbors.Before) != wantLength || len(neighbors.After) != wantLength {
							t.Fatalf("%q: neighbors %d/%d want %d", file.ID, len(neighbors.Before), len(neighbors.After), wantLength)
						}
						for distance := 1; distance <= wantLength; distance++ {
							prev := listing.Files[(index-distance+len(listing.Files))%len(listing.Files)].ID
							next := listing.Files[(index+distance)%len(listing.Files)].ID
							if neighbors.Before[distance-1].ID != prev || neighbors.After[distance-1].ID != next {
								t.Fatalf("anchor %s step %d: got %s/%s want %s/%s", file.ID, distance, neighbors.Before[distance-1].ID, neighbors.After[distance-1].ID, prev, next)
							}
						}
					}
				})
			}
		}
	}
	// A file outside the filtered listing cannot be used as an anchor.
	all := listTestFiles(t, server, "", 20)
	for _, file := range all.Files {
		if file.MediaKind == "other" {
			_, status := requestAround(t, server, file.ID, "kind:image", "added", "desc", 5)
			if status != http.StatusNotFound {
				t.Fatalf("non-member returned status %d", status)
			}
			break
		}
	}
}

func TestFileNeighborsRejectInvalidCount(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()
	for _, count := range []int{0, -1, 21} {
		_, status := requestAround(t, server, "missing", "", "added", "desc", count)
		if status != http.StatusBadRequest {
			t.Fatalf("count %d: status %d", count, status)
		}
	}
}

func TestFileNeighborsWrapPastUnloadedPages(t *testing.T) {
	directory := t.TempDir()
	server, client := newTestBrowseServerAt(t, directory, directory+"/gooru.db")
	defer client.Close()
	paths := make([]string, 0, 65)
	for i := 0; i < 65; i++ {
		paths = append(paths, writeTestFile(t, directory, fmt.Sprintf("extra-%03d.jpg", i), fmt.Sprintf("unique image %d", i)))
	}
	if _, err := client.TagFiles(paths, []string{"kind:image"}, nil, false); err != nil {
		t.Fatal(err)
	}
	list := func(limit int) FileListResponse {
		target := fmt.Sprintf("/api/v1/files?query=kind:image&sort=name&order=asc&limit=%d", limit)
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, authedRequest(http.MethodGet, target))
		if recorder.Code != http.StatusOK {
			t.Fatalf("listing %d: %s", recorder.Code, recorder.Body.String())
		}
		var result FileListResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	firstPage, all := list(60), list(200)
	if len(all.Files) <= len(firstPage.Files) {
		t.Fatal("test library did not span multiple pages")
	}
	first, last := all.Files[0], all.Files[len(all.Files)-1]
	neighbors, status := requestAround(t, server, first.ID, "kind:image", "name", "asc", 5)
	if status != http.StatusOK {
		t.Fatalf("around first: HTTP %d", status)
	}
	if neighbors.Before[0].ID != last.ID || neighbors.Before[0].ID == firstPage.Files[len(firstPage.Files)-1].ID {
		t.Fatalf("previous of first must wrap to last matching file, got %s want %s", neighbors.Before[0].ID, last.ID)
	}
	neighbors, status = requestAround(t, server, last.ID, "kind:image", "name", "asc", 5)
	if status != http.StatusOK || neighbors.After[0].ID != first.ID {
		t.Fatalf("next of last must wrap to first matching file: HTTP %d response %+v", status, neighbors)
	}
}
