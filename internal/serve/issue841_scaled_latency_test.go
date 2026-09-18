package serve

import (
	"context"
	"encoding/json"
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

	extra := make([]string, 0, librarySize-3)
	for index := 0; index < librarySize-3; index++ {
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
		if _, err := client.TagFiles(extra[start:end], []string{"scale:seed"}, nil, false); err != nil {
			t.Fatalf("seed scaled library %d:%d: %v", start, end, err)
		}
	}

	count, err := client.CountFilesByQuery("", false)
	if err != nil {
		t.Fatalf("count scaled library: %v", err)
	}
	if count != librarySize {
		t.Fatalf("scaled library count = %d, want %d", count, librarySize)
	}
	page := listTestFiles(t, server, "", 1)
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
			if elapsed < minDuration { minDuration = elapsed }
			if elapsed > maxDuration { maxDuration = elapsed }
		}
		t.Logf("issue841 scaled run %d: library=%d operations=%d avg=%s min=%s max=%s", run+1, librarySize, operationsPerRun, total/operationsPerRun, minDuration, maxDuration)
	}

	for run := 0; run < runs; run++ {
		var admissionTotal time.Duration
		var queueTotal time.Duration
		var workerTotal time.Duration
		admissionMin := time.Duration(1<<63 - 1)
		queueMin := time.Duration(1<<63 - 1)
		workerMin := time.Duration(1<<63 - 1)
		var admissionMax time.Duration
		var queueMax time.Duration
		var workerMax time.Duration

		for operation := 0; operation < operationsPerRun; operation++ {
			method := http.MethodPost
			if (run*operationsPerRun+operation)%2 == 1 {
				method = http.MethodDelete
			}
			req := authedJSONRequest(method, "/api/v1/files/tags", `{"file_ids":["`+fileID+`"],"tags":["bench"],"verbose":false}`)
			req.Header.Set("Prefer", "respond-async")
			rec := httptest.NewRecorder()
			started := time.Now()
			server.Handler().ServeHTTP(rec, req)
			admissionElapsed := time.Since(started)
			if rec.Code != http.StatusAccepted {
				t.Fatalf("async mutation %d/%d returned %d: %s", run, operation, rec.Code, rec.Body.String())
			}
			var admitted BackgroundOperationDTO
			if err := json.Unmarshal(rec.Body.Bytes(), &admitted); err != nil {
				t.Fatalf("decode async mutation %d/%d: %v", run, operation, err)
			}
			if admitted.ID == "" {
				t.Fatalf("async mutation %d/%d returned no operation id", run, operation)
			}
			if _, err := waitForDurableTagMutation(ctx, server.backgroundOperations, admitted.ID); err != nil {
				t.Fatalf("wait async mutation %d/%d: %v", run, operation, err)
			}
			state, found, err := server.backgroundOperations.GetBackgroundOperation(admitted.ID)
			if err != nil {
				t.Fatalf("read async mutation %d/%d: %v", run, operation, err)
			}
			if !found || state.StartedAt == nil || state.FinishedAt == nil {
				t.Fatalf("async mutation %d/%d has incomplete lifecycle timestamps: %+v", run, operation, state)
			}
			queueElapsed := state.StartedAt.Sub(state.CreatedAt)
			workerElapsed := state.FinishedAt.Sub(*state.StartedAt)

			admissionTotal += admissionElapsed
			queueTotal += queueElapsed
			workerTotal += workerElapsed
			if admissionElapsed < admissionMin { admissionMin = admissionElapsed }
			if admissionElapsed > admissionMax { admissionMax = admissionElapsed }
			if queueElapsed < queueMin { queueMin = queueElapsed }
			if queueElapsed > queueMax { queueMax = queueElapsed }
			if workerElapsed < workerMin { workerMin = workerElapsed }
			if workerElapsed > workerMax { workerMax = workerElapsed }
		}
		t.Logf(
			"issue841 split run %d: library=%d operations=%d admission_avg=%s admission_min=%s admission_max=%s queue_avg=%s queue_min=%s queue_max=%s worker_avg=%s worker_min=%s worker_max=%s",
			run+1,
			librarySize,
			operationsPerRun,
			admissionTotal/operationsPerRun,
			admissionMin,
			admissionMax,
			queueTotal/operationsPerRun,
			queueMin,
			queueMax,
			workerTotal/operationsPerRun,
			workerMin,
			workerMax,
		)
	}

	t.Fatalf("issue 841 diagnostic complete; scaled and split timings are logged above")
}
