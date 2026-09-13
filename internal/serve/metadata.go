package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	PageCount     *int     `json:"page_count,omitempty"`
}

// MediaMetadataProvider extracts metadata from an authenticated logical media
// source. Source support is the minimum provider capability so new providers
// cannot accidentally work only when a plaintext pathname is available.
type MediaMetadataProvider interface {
	MetadataFromSource(ctx context.Context, file types.FileInfo, source io.ReaderAt, size int64, mediaType string, mediaKind string) (MediaMetadata, error)
}

// MediaMetadataPathProvider is an optional clear-mode optimization for tools
// that benefit materially from receiving the real plaintext pathname (for
// example ffprobe). Protected and distinct staging sources never use it.
type MediaMetadataPathProvider interface {
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
	if strings.EqualFold(filepath.Ext(file.Path), ".cbz") {
		return p.metadataFromPathSource(ctx, file, mediaType, mediaKind)
	}
	switch mediaKind {
	case "photo", "gif":
		return p.metadataFromPathSource(ctx, file, mediaType, mediaKind)
	case "video":
		return p.videoMetadata(ctx, file)
	default:
		return MediaMetadata{}, nil
	}
}

func (p BasicMediaMetadataProvider) MetadataFromSource(ctx context.Context, file types.FileInfo, source io.ReaderAt, size int64, mediaType string, mediaKind string) (MediaMetadata, error) {
	if err := ctx.Err(); err != nil {
		return MediaMetadata{}, err
	}
	if source == nil || size < 0 {
		return MediaMetadata{}, nil
	}
	if strings.EqualFold(filepath.Ext(file.Path), ".cbz") {
		archive, err := openComicArchiveReader(file.Path, source, size, nil)
		if err != nil {
			return MediaMetadata{}, err
		}
		defer archive.Close()
		return comicArchiveMetadata(archive)
	}
	reader := io.NewSectionReader(source, 0, size)
	switch mediaKind {
	case "photo", "gif":
		cfg, _, err := image.DecodeConfig(reader)
		if err != nil {
			return MediaMetadata{}, err
		}
		width := cfg.Width
		height := cfg.Height
		return MediaMetadata{ImageWidth: &width, ImageHeight: &height}, nil
	case "video":
		return p.videoMetadataFromReader(ctx, reader)
	default:
		return MediaMetadata{}, nil
	}
}

func (p BasicMediaMetadataProvider) metadataFromPathSource(ctx context.Context, file types.FileInfo, mediaType string, mediaKind string) (MediaMetadata, error) {
	f, err := os.Open(fileStoragePath(file))
	if err != nil {
		return MediaMetadata{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return MediaMetadata{}, err
	}
	if info.IsDir() {
		return MediaMetadata{}, fmt.Errorf("media metadata source is a directory")
	}
	return p.MetadataFromSource(ctx, file, f, info.Size(), mediaType, mediaKind)
}

func comicArchiveMetadata(archive *comicArchive) (MediaMetadata, error) {
	count := len(archive.pages)
	meta := MediaMetadata{PageCount: &count}
	if count == 0 {
		return meta, nil
	}
	reader, _, err := archive.openPage(0)
	if err != nil {
		return MediaMetadata{}, err
	}
	defer reader.Close()
	cfg, _, err := image.DecodeConfig(io.LimitReader(reader, maxComicPageBytes))
	if err != nil {
		return MediaMetadata{}, fmt.Errorf("decode comic cover metadata: %w", err)
	}
	width := cfg.Width
	height := cfg.Height
	meta.ImageWidth = &width
	meta.ImageHeight = &height
	return meta, nil
}

func (p BasicMediaMetadataProvider) videoMetadata(ctx context.Context, file types.FileInfo) (MediaMetadata, error) {
	if strings.TrimSpace(p.FFprobePath) == "" {
		return MediaMetadata{}, nil
	}
	return p.runFFprobe(ctx, fileStoragePath(file), nil)
}

func (p BasicMediaMetadataProvider) videoMetadataFromReader(ctx context.Context, source io.Reader) (MediaMetadata, error) {
	if strings.TrimSpace(p.FFprobePath) == "" {
		return MediaMetadata{}, nil
	}
	return p.runFFprobe(ctx, "pipe:0", source)
}

func (p BasicMediaMetadataProvider) runFFprobe(ctx context.Context, input string, stdin io.Reader) (MediaMetadata, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, p.FFprobePath,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,duration,nb_frames:format=duration",
		"-of", "json",
		input,
	)
	cmd.Stdin = stdin
	out, err := cmd.Output()
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
