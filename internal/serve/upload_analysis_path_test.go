package serve

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

type recordingImportMetadataProvider struct {
	path       string
	mediaType  string
	mediaKind  string
	sourceUsed bool
	sourceSize int64
}

func (p *recordingImportMetadataProvider) Metadata(_ context.Context, file types.FileInfo, mediaType string, mediaKind string) (MediaMetadata, error) {
	p.path = file.Path
	p.mediaType = mediaType
	p.mediaKind = mediaKind
	width, height := 7, 5
	return MediaMetadata{ImageWidth: &width, ImageHeight: &height}, nil
}

func (p *recordingImportMetadataProvider) MetadataFromSource(_ context.Context, file types.FileInfo, source io.ReaderAt, size int64, mediaType string, mediaKind string) (MediaMetadata, error) {
	p.path = file.Path
	p.mediaType = mediaType
	p.mediaKind = mediaKind
	p.sourceUsed = true
	p.sourceSize = size
	probe := make([]byte, 1)
	if _, err := source.ReadAt(probe, 0); err != nil {
		return MediaMetadata{}, err
	}
	width, height := 7, 5
	return MediaMetadata{ImageWidth: &width, ImageHeight: &height}, nil
}

var _ MediaMetadataProvider = (*recordingImportMetadataProvider)(nil)
var _ MediaMetadataPathProvider = (*recordingImportMetadataProvider)(nil)

type sourceOnlyImportMetadataProvider struct {
	sourceUsed bool
}

func (p *sourceOnlyImportMetadataProvider) MetadataFromSource(_ context.Context, _ types.FileInfo, source io.ReaderAt, _ int64, _ string, _ string) (MediaMetadata, error) {
	p.sourceUsed = true
	probe := make([]byte, 1)
	if _, err := source.ReadAt(probe, 0); err != nil {
		return MediaMetadata{}, err
	}
	width := 3
	return MediaMetadata{ImageWidth: &width}, nil
}

var _ MediaMetadataProvider = (*sourceOnlyImportMetadataProvider)(nil)

func TestImportedMediaMetadataUsesDirectPathForClearSamePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(path, []byte("media"), 0600); err != nil {
		t.Fatal(err)
	}
	provider := &recordingImportMetadataProvider{}
	library := &GooruLibrary{}
	file := types.FileInfo{Path: path}

	if _, err := library.importedMediaMetadata(context.Background(), provider, file, path, "video/mp4", "video"); err != nil {
		t.Fatalf("extract clear metadata: %v", err)
	}
	if provider.sourceUsed {
		t.Fatal("clear same-path metadata unexpectedly used the logical source path")
	}
	if provider.path != path {
		t.Fatalf("metadata provider path = %q, want %q", provider.path, path)
	}
	if provider.mediaType != "video/mp4" || provider.mediaKind != "video" {
		t.Fatalf("metadata classification = %q/%q, want video/mp4/video", provider.mediaType, provider.mediaKind)
	}
}

func TestImportedMediaMetadataUsesRequiredSourceWhenPathOptimizationUnavailable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image.png")
	if err := os.WriteFile(path, []byte("media"), 0600); err != nil {
		t.Fatal(err)
	}
	provider := &sourceOnlyImportMetadataProvider{}
	library := &GooruLibrary{}
	file := types.FileInfo{Path: path}

	metadata, err := library.importedMediaMetadata(context.Background(), provider, file, path, "image/png", "photo")
	if err != nil {
		t.Fatalf("extract source-only metadata: %v", err)
	}
	if !provider.sourceUsed {
		t.Fatal("source-only metadata provider was not given the required logical source")
	}
	if metadata.ImageWidth == nil || *metadata.ImageWidth != 3 {
		t.Fatalf("source-only metadata = %+v, want image width", metadata)
	}
}

func TestGooruUploadImportSeparatesAnalysisSourceFromRegisteredDestination(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	writeInitializedTestDB(t, dbPath, types.StrategyFull)
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	defer client.Close()

	source := filepath.Join(dir, "plaintext-staging.tmp")
	data := mustReadFile(t, writePNGImage(t))
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatalf("write analysis source: %v", err)
	}
	destination := filepath.Join(dir, "library", "image.png")
	provider := &recordingImportMetadataProvider{}
	library := NewGooruLibrary(client, false)
	library.metadata = provider

	response, err := library.ImportUploadedFiles(context.Background(), []StagedUpload{{
		Name:         "image.png",
		Path:         destination,
		AnalysisPath: source,
		Size:         int64(len(data)),
		TargetID:     "default",
	}}, []string{"uploaded"})
	if err != nil {
		t.Fatalf("import staged upload: %v", err)
	}
	if len(response.Files) != 1 || response.Files[0].Status != "imported" {
		t.Fatalf("unexpected response: %+v", response.Files)
	}

	registered, err := client.GetFileInfoByPath(destination)
	if err != nil {
		t.Fatalf("registered destination: %v", err)
	}
	if registered.Path != destination {
		t.Fatalf("registered logical destination = %q, want %q", registered.Path, destination)
	}
	if !provider.sourceUsed {
		t.Fatal("metadata provider did not receive the random-access analysis source")
	}
	if provider.path != destination {
		t.Fatalf("metadata provider logical path = %q, want destination %q", provider.path, destination)
	}
	if provider.sourceSize != int64(len(data)) {
		t.Fatalf("metadata source size = %d, want %d", provider.sourceSize, len(data))
	}
	if provider.mediaType != "image/png" || provider.mediaKind != "photo" {
		t.Fatalf("metadata classification = %q/%q, want image/png/photo", provider.mediaType, provider.mediaKind)
	}
	metadata, err := client.GetMediaMetadata(registered.ID)
	if err != nil {
		t.Fatalf("cached metadata: %v", err)
	}
	if metadata.ImageWidth == nil || *metadata.ImageWidth != 7 || metadata.ImageHeight == nil || *metadata.ImageHeight != 5 {
		t.Fatalf("cached metadata = %+v", metadata)
	}
}

func TestRejectedStagedUploadRemovesDistinctAnalysisSource(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "analysis.tmp")
	destination := filepath.Join(dir, "stored.enc")
	if err := os.WriteFile(source, []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("destination"), 0600); err != nil {
		t.Fatal(err)
	}

	removeRejectedStagedUpload(StagedUpload{Path: destination, AnalysisPath: source})
	for _, path := range []string{source, destination} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("rejected staged upload left %s, err=%v", path, err)
		}
	}
}
