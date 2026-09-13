package serve

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/internal/filesource"
	"gooru.local/types"
)

type recordingQualityThumbnailer struct {
	pathCalls   int
	sourceCalls int
	lastPath    string
	lastName    string
	lastQuality int
	sourceBytes []byte
}

func (t *recordingQualityThumbnailer) BackendVersion() string {
	return "quality-policy-test"
}

func (t *recordingQualityThumbnailer) Thumbnail(string, io.Writer, int, string) error {
	return nil
}

func (t *recordingQualityThumbnailer) ThumbnailQuality(path string, _ io.Writer, _ int, _ string, quality int) error {
	t.pathCalls++
	t.lastPath = path
	t.lastQuality = quality
	return nil
}

func (t *recordingQualityThumbnailer) ThumbnailSourceQuality(name string, src io.ReadSeeker, _ io.Writer, _ int, _ string, quality int) error {
	t.sourceCalls++
	t.lastName = name
	t.lastQuality = quality
	data, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	t.sourceBytes = data
	return nil
}

type thumbnailOnlyBackend struct{}

func (thumbnailOnlyBackend) BackendVersion() string { return "thumbnail-only-test" }
func (thumbnailOnlyBackend) Thumbnail(string, io.Writer, int, string) error {
	return nil
}

func TestThumbnailQualityPolicyClearUsesDirectPath(t *testing.T) {
	thumbnailer := &recordingQualityThumbnailer{}
	media := &MediaService{thumbnailer: thumbnailer}
	path := filepath.Join(t.TempDir(), "not-opened.jpg")
	file := types.FileInfo{Path: path}

	handled, err := newThumbnailQualityGenerationPolicy(false)(media, file, io.Discard, 64, "jpeg", 81)
	if err != nil {
		t.Fatalf("generate clear quality thumbnail: %v", err)
	}
	if !handled {
		t.Fatal("expected quality-capable clear backend to handle preview")
	}
	if thumbnailer.pathCalls != 1 || thumbnailer.sourceCalls != 0 {
		t.Fatalf("quality calls: path=%d source=%d, want path=1 source=0", thumbnailer.pathCalls, thumbnailer.sourceCalls)
	}
	if thumbnailer.lastPath != path {
		t.Fatalf("quality path = %q, want %q", thumbnailer.lastPath, path)
	}
	if thumbnailer.lastQuality != 81 {
		t.Fatalf("quality = %d, want 81", thumbnailer.lastQuality)
	}
}

func TestThumbnailQualityPolicyProtectedUsesLogicalSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "photo.jpg")
	payload := []byte("logical media bytes")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write logical media: %v", err)
	}
	thumbnailer := &recordingQualityThumbnailer{}
	media := &MediaService{
		thumbnailer:    thumbnailer,
		sourceResolver: filesource.NewFilesystem(),
	}
	file := types.FileInfo{Path: path}

	handled, err := newThumbnailQualityGenerationPolicy(true)(media, file, io.Discard, 64, "jpeg", 77)
	if err != nil {
		t.Fatalf("generate protected quality thumbnail: %v", err)
	}
	if !handled {
		t.Fatal("expected source-quality backend to handle protected preview")
	}
	if thumbnailer.pathCalls != 0 || thumbnailer.sourceCalls != 1 {
		t.Fatalf("quality calls: path=%d source=%d, want path=0 source=1", thumbnailer.pathCalls, thumbnailer.sourceCalls)
	}
	if thumbnailer.lastName != path {
		t.Fatalf("logical name = %q, want %q", thumbnailer.lastName, path)
	}
	if string(thumbnailer.sourceBytes) != string(payload) {
		t.Fatalf("logical source bytes = %q, want %q", thumbnailer.sourceBytes, payload)
	}
	if thumbnailer.lastQuality != 77 {
		t.Fatalf("quality = %d, want 77", thumbnailer.lastQuality)
	}
}

func TestThumbnailQualityPolicyProtectedSkipsOpenWhenBackendCannotOverrideQuality(t *testing.T) {
	media := &MediaService{thumbnailer: thumbnailOnlyBackend{}}
	file := types.FileInfo{Path: filepath.Join(t.TempDir(), "missing.jpg")}

	handled, err := newThumbnailQualityGenerationPolicy(true)(media, file, io.Discard, 64, "jpeg", 75)
	if err != nil {
		t.Fatalf("unsupported quality policy: %v", err)
	}
	if handled {
		t.Fatal("expected ordinary thumbnail fallback when backend has no source-quality capability")
	}
}
