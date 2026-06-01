package serve

import (
	"context"
	"encoding/json"
	"image"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

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

type BasicMediaMetadataProvider struct {
	FFprobePath string
}

func NewMediaMetadataProvider(cfg Config) BasicMediaMetadataProvider {
	return BasicMediaMetadataProvider{FFprobePath: resolveCommandPath(cfg.Tools.FFprobePath)}
}

func (p BasicMediaMetadataProvider) Metadata(ctx context.Context, file types.FileInfo, mediaType string, mediaKind string) (MediaMetadata, error) {
	if err := ctx.Err(); err != nil {
		return MediaMetadata{}, err
	}
	switch mediaKind {
	case "photo", "gif":
		return p.imageMetadata(ctx, file)
	case "video":
		return p.videoMetadata(ctx, file)
	default:
		return MediaMetadata{}, nil
	}
}

func (p BasicMediaMetadataProvider) imageMetadata(ctx context.Context, file types.FileInfo) (MediaMetadata, error) {
	if err := ctx.Err(); err != nil {
		return MediaMetadata{}, err
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

func (p BasicMediaMetadataProvider) videoMetadata(ctx context.Context, file types.FileInfo) (MediaMetadata, error) {
	if strings.TrimSpace(p.FFprobePath) == "" {
		return MediaMetadata{}, nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(probeCtx, p.FFprobePath,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,duration,nb_frames:format=duration",
		"-of", "json",
		file.Path,
	).Output()
	if err := ctx.Err(); err != nil {
		return MediaMetadata{}, err
	}
	if err != nil {
		return MediaMetadata{}, nil
	}
	var payload struct {
		Streams []struct {
			Width    int    `json:"width"`
			Height   int    `json:"height"`
			Duration string `json:"duration"`
			Frames   string `json:"nb_frames"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return MediaMetadata{}, nil
	}
	meta := MediaMetadata{}
	if len(payload.Streams) > 0 {
		stream := payload.Streams[0]
		if stream.Width > 0 {
			meta.VideoWidth = &stream.Width
		}
		if stream.Height > 0 {
			meta.VideoHeight = &stream.Height
		}
		if frames, ok := parsePositiveInt(stream.Frames); ok {
			meta.FrameCount = &frames
		}
		if seconds, ok := parsePositiveFloat(stream.Duration); ok {
			meta.VideoDuration = &seconds
		}
	}
	if meta.VideoDuration == nil {
		if seconds, ok := parsePositiveFloat(payload.Format.Duration); ok {
			meta.VideoDuration = &seconds
		}
	}
	return meta, nil
}

func parsePositiveFloat(raw string) (float64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	return value, err == nil && value > 0
}

func parsePositiveInt(raw string) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	return value, err == nil && value > 0
}
