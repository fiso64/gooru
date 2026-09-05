package serve

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func TestServerMediaRuntimeConfigDoesNotRetainEncryptionKey(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x42}, 32)
	cfg.Uploads.Targets = []UploadTarget{{ID: "managed", Path: t.TempDir()}}

	server := NewServerWithLibrary(cfg, nil)
	if len(server.media.cfg.Encryption.Key) != 0 {
		t.Fatal("media runtime config retained raw encryption key material")
	}
	if !server.media.cfg.Encryption.Enabled {
		t.Fatal("media runtime config lost protected-mode behavior flag")
	}
	if server.media.sourceResolver == nil {
		t.Fatal("media service did not receive composed logical source resolver")
	}
}

type thumbnailAccessRecorder struct {
	pathCalls   int
	sourceCalls int
	sourceBody  string
}

func (r *thumbnailAccessRecorder) BackendVersion() string { return "access-recorder" }

func (r *thumbnailAccessRecorder) Thumbnail(_ string, dst io.Writer, _ int, _ string) error {
	r.pathCalls++
	_, err := io.WriteString(dst, "path")
	return err
}

func (r *thumbnailAccessRecorder) ThumbnailSource(_ string, src io.ReadSeeker, dst io.Writer, _ int, _ string) error {
	r.sourceCalls++
	body, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	r.sourceBody = string(body)
	_, err = io.WriteString(dst, "source")
	return err
}

func TestThumbnailAccessPolicyIsSelectedAtCompositionBoundary(t *testing.T) {
	t.Run("ordinary mode keeps path backend fast path", func(t *testing.T) {
		cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
		media := NewMediaService(cfg)
		recorder := &thumbnailAccessRecorder{}
		media.thumbnailer = recorder

		var dst bytes.Buffer
		if err := media.generateThumbnail(types.FileInfo{Path: "logical.jpg"}, &dst, 64, "jpeg"); err != nil {
			t.Fatalf("generate thumbnail: %v", err)
		}
		if recorder.pathCalls != 1 || recorder.sourceCalls != 0 {
			t.Fatalf("path/source calls = %d/%d, want 1/0", recorder.pathCalls, recorder.sourceCalls)
		}
	})

	t.Run("protected mode forces logical source backend", func(t *testing.T) {
		root := t.TempDir()
		external := filepath.Join(t.TempDir(), "external.jpg")
		if err := os.WriteFile(external, []byte("logical plaintext"), 0600); err != nil {
			t.Fatalf("write external source: %v", err)
		}

		cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
		cfg.Encryption.Enabled = true
		cfg.Encryption.Key = bytes.Repeat([]byte{0x42}, 32)
		cfg.Uploads.Targets = []UploadTarget{{ID: "managed", Path: root}}
		media := NewMediaService(cfg)
		recorder := &thumbnailAccessRecorder{}
		media.thumbnailer = recorder

		var dst bytes.Buffer
		if err := media.generateThumbnail(types.FileInfo{Path: external}, &dst, 64, "jpeg"); err != nil {
			t.Fatalf("generate protected thumbnail: %v", err)
		}
		if recorder.pathCalls != 0 || recorder.sourceCalls != 1 {
			t.Fatalf("path/source calls = %d/%d, want 0/1", recorder.pathCalls, recorder.sourceCalls)
		}
		if recorder.sourceBody != "logical plaintext" {
			t.Fatalf("source body = %q, want logical plaintext", recorder.sourceBody)
		}
	})
}
