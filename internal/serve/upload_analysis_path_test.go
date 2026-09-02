package serve

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

type recordingImportMetadataProvider struct {
	path      string
	mediaType string
	mediaKind string
}

func (p *recordingImportMetadataProvider) Metadata(_ context.Context, file types.FileInfo, mediaType string, mediaKind string) (MediaMetadata, error) {
	p.path = file.Path
	p.mediaType = mediaType
	p.mediaKind = mediaKind
	width, height := 7, 5
	return MediaMetadata{ImageWidth: &width, ImageHeight: &height}, nil
}

func TestGooruUploadImportSeparatesAnalysisSourceFromRegisteredDestination(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	if err := core.Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	defer client.Close()

	source := filepath.Join(dir, "plaintext-staging.tmp")
	if err := os.WriteFile(source, mustReadFile(t, writePNGImage(t)), 0600); err != nil {
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
		Size:         int64(len(mustReadFile(t, source))),
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
	if provider.path != source {
		t.Fatalf("metadata provider path = %q, want analysis source %q", provider.path, source)
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
