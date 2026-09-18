package serve

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func BenchmarkTagMutationSyncSingleFile(b *testing.B) {
	dir := b.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	server, client := newTestBrowseServerAt(b, dir, dbPath)
	defer client.Close()
	runtime, err := server.NewBackgroundRuntime(client, "tag-mutation-bench")
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
			b.Fatalf("stop background runtime: %v", err)
		}
	}()

	page := listTestFiles(b, server, "kind:image", 1)
	fileID := page.Files[0].ID
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		method := http.MethodPost
		if i%2 == 1 {
			method = http.MethodDelete
		}
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, authedJSONRequest(method, "/api/v1/files/tags", `{"file_ids":["`+fileID+`"],"tags":["bench"]}`))
		if rec.Code != http.StatusOK {
			b.Fatalf("mutation %d returned %d: %s", i, rec.Code, rec.Body.String())
		}
	}
}
