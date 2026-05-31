package serve

import (
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

func writeJSONFFprobe(t *testing.T, output string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ffprobe")
	body := "#!/bin/sh\nif [ \"$1\" = \"-version\" ]; then echo \"fake ffprobe 1.0\"; exit 0; fi\ncat <<'JSON'\n" + output + "\nJSON\n"
	if err := os.WriteFile(path, []byte(body), 0700); err != nil {
		t.Fatalf("write fake ffprobe: %v", err)
	}
	return path
}
