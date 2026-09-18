package serve

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestIssue841ScaledTagLatency(t *testing.T) {
	const librarySize = 1340
	const operationsPerRun = 20
	const runs = 3

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	server, client := newTestBrowseServerAt(t, dir, dbPath)
	defer client.Close()

	extra := make([]string, 0, librarySize-2)
	for index := 0; index < librarySize-2; index++ {
		ext := ".jpg"
		switch {
		case index >= 500 && index < 1240:
			ext = ".mp4"
		case index >= 1240:
			ext = ".gif"
		}
		extra = append(extra, writeTestFile(t, dir, fmt.Sprintf("scale-%04d%s", index, ext), fmt.Sprintf("issue-841-scale-payload-%04d", index)))
	}
	for start := 0; start < len(extra); start += 100 {
		end := min(start+100, len(extra))
		if _, err := client.TagFiles(extra[start:end], []string{"kind:image", "scale:seed"}, nil, false); err != nil {
			t.Fatalf("seed scaled library %d:%d: %v", start, end, err)
		}
	}

	page := listTestFiles(t, server, "kind:image", 1)
	if page.TotalCount != librarySize {
		t.Fatalf("scaled library count = %d, want %d", page.TotalCount, librarySize)
	}
	fileID := page.Files[0].ID

	runtime, err := server.NewBackgroundRuntime(client, "tag-mutation-scale-bench")
	if err != nil {
		t.Fatal(err)
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

	for run := 0; run < runs; run++ {
		var total time.Duration
		minDuration := time.Duration(1<<63 - 1)
		var maxDuration time.Duration
		for operation := 0; operation < operationsPerRun; operation++ {
			method := http.MethodPost
			if (run*operationsPerRun+operation)%2 == 1 {
				method = http.MethodDelete
			}
			rec := httptest.NewRecorder()
			started := time.Now()
			server.Handler().ServeHTTP(rec, authedJSONRequest(method, "/api/v1/files/tags", `{"file_ids":["`+fileID+`"],"tags":["bench"],"verbose":false}`))
			elapsed := time.Since(started)
			if rec.Code != http.StatusOK {
				t.Fatalf("mutation %d/%d returned %d: %s", run, operation, rec.Code, rec.Body.String())
			}
			total += elapsed
			if elapsed < minDuration {
				minDuration = elapsed
			}
			if elapsed > maxDuration {
				maxDuration = elapsed
			}
		}
		t.Logf("issue841 scaled run %d: library=%d operations=%d avg=%s min=%s max=%s", run+1, librarySize, operationsPerRun, total/operationsPerRun, minDuration, maxDuration)
	}
	t.Fatalf("issue 841 diagnostic complete; scaled timings are logged above")
}
