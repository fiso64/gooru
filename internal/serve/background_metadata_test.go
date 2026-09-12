package serve

import (
	"context"
	"testing"

	"gooru.local/types"
)

type countingMediaMetadataProvider struct {
	calls int
}

func (p *countingMediaMetadataProvider) Metadata(context.Context, types.FileInfo, string, string) (MediaMetadata, error) {
	p.calls++
	width := 1
	return MediaMetadata{ImageWidth: &width}, nil
}

func TestDeferredUploadMediaMetadataSkipsSynchronousExtraction(t *testing.T) {
	provider := &countingMediaMetadataProvider{}
	library := &GooruLibrary{}
	file := types.FileInfo{Path: "/library/photo.jpg"}

	metadata, err := library.importedMediaMetadata(withDeferredUploadMediaMetadata(context.Background()), provider, file, "", "image/jpeg", "photo")
	if err != nil {
		t.Fatalf("deferred metadata: %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("durable upload metadata provider calls = %d, want 0", provider.calls)
	}
	if metadata.ImageWidth != nil {
		t.Fatalf("deferred metadata should be empty, got %+v", metadata)
	}

	metadata, err = library.importedMediaMetadata(context.Background(), provider, file, "", "image/jpeg", "photo")
	if err != nil {
		t.Fatalf("direct metadata: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("direct upload metadata provider calls = %d, want 1", provider.calls)
	}
	if metadata.ImageWidth == nil || *metadata.ImageWidth != 1 {
		t.Fatalf("direct metadata = %+v, want image width", metadata)
	}
}

func TestBackgroundUploadTaskRequestsAlwaysIncludeMetadata(t *testing.T) {
	cfg := DefaultConfig("")
	cfg.Media.ThumbnailSizes = nil
	media := NewMediaService(cfg)
	location := types.LocationInfo{Path: "/library/photo.jpg", Hash: "content-hash"}

	tasks := media.backgroundUploadTaskRequests(location)
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want metadata task only", len(tasks))
	}
	task := tasks[0]
	if task.Kind != backgroundMediaMetadataTaskKind || task.SubjectKind != "location" || task.SubjectID != location.Path {
		t.Fatalf("unexpected metadata task: %+v", task)
	}
	if task.DedupeKey != "metadata:"+location.Path || task.InputKey != location.Hash || task.ResourceClass != backgroundThumbnailResourceClass {
		t.Fatalf("unexpected metadata task identity: %+v", task)
	}
}
