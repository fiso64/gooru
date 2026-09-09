package serve

import (
	"slices"
	"testing"
	"time"
)

func TestFFmpegThumbnailArgsBoundDecodeAndEncodeThreads(t *testing.T) {
	t.Parallel()

	offset := 2 * time.Second
	for name, args := range map[string][]string{
		"path":   ffmpegThumbnailArgs("input.mp4", "thumb.jpg", 320, "jpeg", &offset),
		"source": ffmpegThumbnailSourceArgs(320, "jpeg", &offset),
	} {
		t.Run(name, func(t *testing.T) {
			threadOptions := 0
			for i := 0; i+1 < len(args); i++ {
				if args[i] == "-threads" && args[i+1] == "1" {
					threadOptions++
				}
			}
			if threadOptions != 2 {
				t.Fatalf("ffmpeg args contain %d single-thread bounds, want decoder and encoder bounds: %v", threadOptions, args)
			}
			if !slices.Contains(args, "-i") {
				t.Fatalf("ffmpeg args are missing input declaration: %v", args)
			}
		})
	}
}
