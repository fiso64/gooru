package serve

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

func TestBackgroundThumbnailHandlerCompletesWhenContentWasRemoved(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	writeInitializedTestDB(t, dbPath, types.StrategyFull)
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	server := NewServerWithLibrary(DefaultConfig(dbPath), NewGooruLibrary(client, false))
	task := core.BackgroundTask{
		Kind:        backgroundThumbnailTaskKind,
		SubjectKind: "content",
		SubjectID:   "deleted-content",
	}
	if err := server.backgroundThumbnailHandler(context.Background(), task); err != nil {
		t.Fatalf("obsolete thumbnail task should complete without retry: %v", err)
	}
}

func TestBackgroundThumbnailHandlerPropagatesLookupFailure(t *testing.T) {
	lookupErr := errors.New("lookup unavailable")
	server := NewServer(DefaultConfig(""))
	server.backgroundContent = failingContentHashLibrary{err: lookupErr}
	task := core.BackgroundTask{
		Kind:        backgroundThumbnailTaskKind,
		SubjectKind: "content",
		SubjectID:   "content-hash",
	}
	if err := server.backgroundThumbnailHandler(context.Background(), task); !errors.Is(err, lookupErr) {
		t.Fatalf("handler error = %v, want %v", err, lookupErr)
	}
}

type failingContentHashLibrary struct {
	err error
}

func (f failingContentHashLibrary) GetFileByContentHash(context.Context, string) (types.FileInfo, error) {
	return types.FileInfo{}, f.err
}
