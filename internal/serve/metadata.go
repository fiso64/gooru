package serve

import (
	"context"
	"image"
	"os"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"gooru.local/types"
)

type MediaMetadata struct {
	ImageWidth    *int     `json:"image_width,omitempty"`
	ImageHeight   *int     `json:"image_height,omitempty"`
	VideoWidth    *int     `json:"video_width,omitempty"`
	VideoHeight   *int     `json:"video_height,omitempty"`
	VideoDuration *float64 `json:"video_duration,omitempty"`
	AudioDuration *float64 `json:"audio_duration,omitempty"`
	FrameCount    *int     `json:"frame_count,omitempty"`
}

type MediaMetadataProvider interface {
	Metadata(ctx context.Context, file types.FileInfo, mediaType string, mediaKind string) (MediaMetadata, error)
}

type BasicMediaMetadataProvider struct{}

func (BasicMediaMetadataProvider) Metadata(ctx context.Context, file types.FileInfo, mediaType string, mediaKind string) (MediaMetadata, error) {
	if err := ctx.Err(); err != nil {
		return MediaMetadata{}, err
	}
	if mediaKind != "photo" && mediaKind != "gif" {
		return MediaMetadata{}, nil
	}
	f, err := os.Open(file.Path)
	if err != nil {
		return MediaMetadata{}, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return MediaMetadata{}, err
	}
	width := cfg.Width
	height := cfg.Height
	return MediaMetadata{ImageWidth: &width, ImageHeight: &height}, nil
}
