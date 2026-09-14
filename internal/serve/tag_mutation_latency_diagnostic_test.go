package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const tagMutationLatencyDiagnosticEnv = "GOORU_TAG_MUTATION_DIAGNOSTIC"

func TestTagMutationLatencyDiagnostic(t *testing.T) {
	if os.Getenv(tagMutationLatencyDiagnosticEnv) != "1" {
		t.Skip("maintainer-only latency diagnostic")
	}

	for _, targetCount := range []int{1, 28} {
		for _, selector := range []string{"explicit", "query"} {
			for _, tagState := range []string{"existing", "new"} {
				for repetition := 1; repetition <= 3; repetition++ {
					elapsed := runTagMutationLatencyScenario(t, targetCount, selector, tagState)
					t.Logf("TAG_MUTATION_LATENCY targets=%d selector=%s tag=%s repetition=%d elapsed=%s", targetCount, selector, tagState, repetition, elapsed)
				}
			}
		}
	}
}

func runTagMutationLatencyScenario(t *testing.T, targetCount int, selector string, tagState string) time.Duration {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	server, client := newTestBrowseServerAt(t, dir, dbPath)
	defer client.Close()

	const extraFiles = 92 // newTestBrowseServerAt creates three files; reproduce the reported 95-file library.
	paths := make([]string, 0, extraFiles)
	for index := 0; index < extraFiles; index++ {
		path := filepath.Join(dir, fmt.Sprintf("bench-%03d.jpg", index))
		if err := os.WriteFile(path, []byte(fmt.Sprintf("benchmark file %03d", index)), 0600); err != nil {
			t.Fatalf("write benchmark file %d: %v", index, err)
		}
		paths = append(paths, path)
	}
	if _, err := client.TagFiles(paths, []string{"bench-library:yes"}, nil, false); err != nil {
		t.Fatalf("seed benchmark library: %v", err)
	}
	if _, err := client.TagFiles(paths[:targetCount], []string{"bench-target:yes"}, nil, false); err != nil {
		t.Fatalf("seed benchmark target set: %v", err)
	}
	if _, err := client.TagFiles(paths[len(paths)-1:], []string{"existing-tag"}, nil, false); err != nil {
		t.Fatalf("seed existing tag: %v", err)
	}

	runtime, err := server.NewBackgroundRuntime(client, fmt.Sprintf("tag-mutation-latency-%d-%s-%s", targetCount, selector, tagState))
	if err != nil {
		t.Fatalf("create background runtime: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("stop background runtime: %v", err)
		}
	}()

	requestBody := map[string]any{
		"tags": []string{"new-tag"},
	}
	if tagState == "existing" {
		requestBody["tags"] = []string{"existing-tag"}
	}

	switch selector {
	case "explicit":
		page := listTestFiles(t, server, "bench-target:yes", 100)
		if len(page.Files) != targetCount {
			t.Fatalf("explicit target count = %d, want %d", len(page.Files), targetCount)
		}
		fileIDs := make([]string, 0, len(page.Files))
		for _, file := range page.Files {
			fileIDs = append(fileIDs, file.ID)
		}
		requestBody["file_ids"] = fileIDs
	case "query":
		requestBody["query"] = "bench-target:yes"
	default:
		t.Fatalf("unknown selector %q", selector)
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("marshal mutation request: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := authedJSONRequest(http.MethodPost, "/api/v1/files/tags", string(body))
	started := time.Now()
	server.Handler().ServeHTTP(recorder, request)
	elapsed := time.Since(started)
	if recorder.Code != http.StatusOK {
		t.Fatalf("mutation status = %d: %s", recorder.Code, recorder.Body.String())
	}
	return elapsed
}
