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

func TestBackgroundMediaMetadataTaskBelongsToUploadOperation(t *testing.T) {
	request := backgroundMediaMetadataTaskRequest("operation-id")
	if request.OperationID != "operation-id" || request.SubjectKind != "operation" || request.SubjectID != "operation-id" {
		t.Fatalf("unexpected finalizer ownership: %+v", request)
	}
	if request.Kind != backgroundMediaMetadataTaskKind || request.ResourceClass != backgroundThumbnailResourceClass {
		t.Fatalf("unexpected finalizer routing: %+v", request)
	}
	if request.DedupeKey != "operation-id:metadata-finalize" {
		t.Fatalf("unexpected finalizer dedupe key: %q", request.DedupeKey)
	}
}
