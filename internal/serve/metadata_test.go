package serve

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func TestVideoMetadataUsesFFprobeJSON(t *testing.T) {
	ffprobe := writeJSONFFprobe(t, `{"streams":[{"width":1920,"height":1080,"duration":"12.5","nb_frames":"300"}],"format":{"duration":"13.0"}}`)
	provider := BasicMediaMetadataProvider{FFprobePath: ffprobe}
	video := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(video, []byte("not a real video; fake ffprobe ignores input"), 0600); err != nil {
		t.Fatalf("write video: %v", err)
	}

	meta, err := provider.Metadata(context.Background(), types.FileInfo{Path: video}, "video/mp4", "video")
	if err != nil {
		t.Fatalf("metadata: %v", err)
	}
	assertVideoMetadata(t, meta)
}

func TestImageMetadataFromRandomAccessSource(t *testing.T) {
	provider := BasicMediaMetadataProvider{}
	path := writePNGImage(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	logical := filepath.Join(t.TempDir(), "encrypted", "image.png")

	meta, err := provider.MetadataFromSource(context.Background(), types.FileInfo{Path: logical}, bytes.NewReader(data), int64(len(data)), "image/png", "photo")
	if err != nil {
		t.Fatalf("metadata from source: %v", err)
	}
	if meta.ImageWidth == nil || meta.ImageHeight == nil || *meta.ImageWidth <= 0 || *meta.ImageHeight <= 0 {
		t.Fatalf("expected image dimensions from source, got %+v", meta)
	}
	if _, err := os.Stat(logical); !os.IsNotExist(err) {
		t.Fatalf("source metadata unexpectedly required logical plaintext path: %v", err)
	}
}

func TestVideoMetadataFromRandomAccessSourceUsesFFprobePipe(t *testing.T) {
	ffprobe := writeSourceJSONFFprobe(t, `{"streams":[{"width":1920,"height":1080,"duration":"12.5","nb_frames":"300"}],"format":{"duration":"13.0"}}`)
	provider := BasicMediaMetadataProvider{FFprobePath: ffprobe}
	data := []byte("authenticated plaintext video bytes")

	meta, err := provider.MetadataFromSource(context.Background(), types.FileInfo{Path: "/logical/encrypted.mp4"}, bytes.NewReader(data), int64(len(data)), "video/mp4", "video")
	if err != nil {
		t.Fatalf("metadata from source: %v", err)
	}
	assertVideoMetadata(t, meta)
}

func TestVideoMetadataDegradesWhenFFprobeMissing(t *testing.T) {
	provider := BasicMediaMetadataProvider{FFprobePath: filepath.Join(t.TempDir(), "missing-ffprobe")}
	meta, err := provider.Metadata(context.Background(), types.FileInfo{Path: "missing.mp4"}, "video/mp4", "video")
	if err != nil {
		t.Fatalf("metadata should degrade without ffprobe: %v", err)
	}
	if meta.VideoWidth != nil || meta.VideoHeight != nil || meta.VideoDuration != nil || meta.FrameCount != nil {
		t.Fatalf("expected empty metadata when ffprobe is unavailable, got %+v", meta)
	}
}

func assertVideoMetadata(t *testing.T, meta MediaMetadata) {
	t.Helper()
	if meta.VideoWidth == nil || *meta.VideoWidth != 1920 {
		t.Fatalf("expected video width, got %+v", meta)
	}
	if meta.VideoHeight == nil || *meta.VideoHeight != 1080 {
		t.Fatalf("expected video height, got %+v", meta)
	}
	if meta.VideoDuration == nil || *meta.VideoDuration != 12.5 {
		t.Fatalf("expected video duration, got %+v", meta)
	}
	if meta.FrameCount == nil || *meta.FrameCount != 300 {
		t.Fatalf("expected frame count, got %+v", meta)
	}
}

func writeJSONFFprobe(t *testing.T, output string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ffprobe")
	body := "#!/bin/sh\nif [ \"$1\" = \"-version\" ]; then echo \"fake ffprobe 1.0\"; exit 0; fi\ncat <<'JSON'\n" + output + "\nJSON\n"
	if err := os.WriteFile(path, []byte(body), 0700); err != nil {
		t.Fatalf("write fake ffprobe: %v", err)
	}
	return path
}

func writeSourceJSONFFprobe(t *testing.T, output string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ffprobe")
	body := "#!/bin/sh\nlast=\"\"\nfor arg in \"$@\"; do last=\"$arg\"; done\n[ \"$last\" = \"pipe:0\" ] || exit 2\ncat >/dev/null\ncat <<'JSON'\n" + output + "\nJSON\n"
	if err := os.WriteFile(path, []byte(body), 0700); err != nil {
		t.Fatalf("write fake source ffprobe: %v", err)
	}
	return path
}
