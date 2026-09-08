package serve

import (
	"context"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

func TestBackgroundThumbnailTaskUsesBrowsingDerivativeIdentity(t *testing.T) {
	cfg := DefaultConfig("")
	cfg.Media.ThumbnailSizes = []int{320, 640}
	cfg.Media.ThumbnailFormat = "jpeg"
	media := NewMediaService(cfg)
	location := types.LocationInfo{Path: "/library/photo.jpg", Hash: "content-hash"}

	tasks := media.backgroundTaskRequests(location)
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(tasks))
	}
	_, _, expected, ok := media.browsingThumbnailSpec(types.FileInfo{Path: location.Path, Hash: location.Hash})
	if !ok {
		t.Fatal("browsing thumbnail unexpectedly unsupported")
	}
	task := tasks[0]
	if task.InputKey != expected || task.DedupeKey != "thumbnail:"+expected || task.SubjectID != location.Hash || task.ResourceClass != backgroundThumbnailResourceClass {
		t.Fatalf("unexpected task: %+v", task)
	}
}

func TestBackgroundThumbnailHandlerUsesCurrentContentLocation(t *testing.T) {
	cfg := DefaultConfig("")
	cfg.Media.ThumbnailSizes = nil
	server := NewServer(cfg)
	server.backgroundContent = fakeContentHashLibrary{file: types.FileInfo{Path: "/moved/photo.jpg", Hash: "hash"}}
	task := core.BackgroundTask{Kind: backgroundThumbnailTaskKind, SubjectKind: "content", SubjectID: "hash"}
	if err := server.backgroundThumbnailHandler(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}
}

type fakeContentHashLibrary struct{ file types.FileInfo }

func (f fakeContentHashLibrary) GetFileByContentHash(context.Context, string) (types.FileInfo, error) {
	return f.file, nil
}
