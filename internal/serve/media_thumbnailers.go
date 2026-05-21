package serve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type UnsupportedMediaError struct {
	Backend string
	Reason  string
	Err     error
}

func (e *UnsupportedMediaError) Error() string {
	if e.Err != nil {
		return e.Backend + ": " + e.Reason + ": " + e.Err.Error()
	}
	return e.Backend + ": " + e.Reason
}

func (e *UnsupportedMediaError) Unwrap() error {
	return e.Err
}

func (e *UnsupportedMediaError) Is(target error) bool {
	return target == ErrUnsupportedMedia
}

func (e *UnsupportedMediaError) Details() map[string]string {
	details := map[string]string{
		"backend": e.Backend,
		"reason":  e.Reason,
	}
	if e.Err != nil {
		details["error"] = e.Err.Error()
	}
	return details
}

type MediaThumbnailer struct {
	imagePrimary  Thumbnailer
	imageFallback Thumbnailer
	video         Thumbnailer
	version       string
}

func NewMediaThumbnailer(cfg Config) Thumbnailer {
	imagePrimary := newPrimaryImageThumbnailer()
	imageFallback := GoImageThumbnailer{}
	video := NewFFmpegVideoThumbnailer(cfg.Tools.FFmpegPath, cfg.Tools.FFprobePath)
	return &MediaThumbnailer{
		imagePrimary:  imagePrimary,
		imageFallback: imageFallback,
		video:         video,
		version: strings.Join([]string{
			"media-chain-v1",
			imagePrimary.BackendVersion(),
			imageFallback.BackendVersion(),
			video.BackendVersion(),
		}, "|"),
	}
}

func (t *MediaThumbnailer) BackendVersion() string {
	return t.version
}

func (t *MediaThumbnailer) Thumbnail(src string, dst io.Writer, size int, format string) error {
	kind := mediaKindForType(mediaTypeForPath(src))
	switch kind {
	case "image":
		if t.imagePrimary != nil {
			if err := t.imagePrimary.Thumbnail(src, dst, size, format); err == nil {
				return nil
			}
		}
		if t.imageFallback != nil {
			return t.imageFallback.Thumbnail(src, dst, size, format)
		}
		return &UnsupportedMediaError{Backend: "image", Reason: "no image thumbnail backend is configured", Err: ErrUnsupportedMedia}
	case "video":
		if t.video == nil {
			return &UnsupportedMediaError{Backend: "ffmpeg", Reason: "video thumbnail backend is not configured", Err: ErrUnsupportedMedia}
		}
		return t.video.Thumbnail(src, dst, size, format)
	default:
		return &UnsupportedMediaError{Backend: "media", Reason: "media kind " + kind + " is not thumbnailable", Err: ErrUnsupportedMedia}
	}
}

type commandThumbnailer struct {
	backend string
	path    string
	version string
	run     func(ctx context.Context, path string, src string, dst string, size int, format string) error
}

func newCommandThumbnailer(backend string, path string, versionArgs []string, run func(context.Context, string, string, string, int, string) error) commandThumbnailer {
	path = strings.TrimSpace(path)
	if path == "" {
		return commandThumbnailer{backend: backend, version: backend + ":missing"}
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return commandThumbnailer{backend: backend, path: path, version: backend + ":missing"}
	}
	return commandThumbnailer{
		backend: backend,
		path:    resolved,
		version: backend + ":" + commandVersion(resolved, versionArgs),
		run:     run,
	}
}

func (t commandThumbnailer) BackendVersion() string {
	return t.version
}

func (t commandThumbnailer) Thumbnail(src string, dst io.Writer, size int, format string) error {
	if t.path == "" || t.run == nil {
		return &UnsupportedMediaError{Backend: t.backend, Reason: "backend executable is not available", Err: ErrUnsupportedMedia}
	}
	ext := derivativeExtension(format)
	tmp, err := os.CreateTemp("", "gooru-"+t.backend+"-*."+ext)
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	defer os.Remove(tmpPath)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := t.run(ctx, t.path, src, tmpPath, size, format); err != nil {
		if errors.Is(err, ErrUnsupportedMedia) {
			return err
		}
		return &UnsupportedMediaError{Backend: t.backend, Reason: "backend failed to generate derivative", Err: err}
	}
	out, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(dst, out)
	return err
}

func NewFFmpegVideoThumbnailer(ffmpegPath string, ffprobePath string) Thumbnailer {
	ffmpeg := newCommandThumbnailer("ffmpeg", ffmpegPath, []string{"-version"}, func(ctx context.Context, exe string, src string, dst string, size int, format string) error {
		vcodec := "mjpeg"
		if format == "png" {
			vcodec = "png"
		}
		scale := fmt.Sprintf("scale=if(gte(iw\\,ih)\\,min(%d\\,iw)\\,-2):if(gte(ih\\,iw)\\,min(%d\\,ih)\\,-2)", size, size)
		args := []string{
			"-v", "error",
			"-y",
			"-i", src,
			"-frames:v", "1",
			"-vf", scale,
			"-f", "image2",
			"-vcodec", vcodec,
			dst,
		}
		return commandError(exec.CommandContext(ctx, exe, args...).Run())
	})
	ffprobeVersion := commandThumbnailer{backend: "ffprobe", path: strings.TrimSpace(ffprobePath), version: "ffprobe:missing"}
	if ffprobeVersion.path != "" {
		if resolved, err := exec.LookPath(ffprobeVersion.path); err == nil {
			ffprobeVersion.path = resolved
			ffprobeVersion.version = "ffprobe:" + commandVersion(resolved, []string{"-version"})
		}
	}
	ffmpeg.version = ffmpeg.version + "|" + ffprobeVersion.version
	return ffmpeg
}

func commandError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &UnsupportedMediaError{Backend: "command", Reason: "backend timed out", Err: err}
	}
	return err
}

func commandVersion(path string, args []string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
	if err != nil {
		return "unknown"
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if line == "" {
		return "unknown"
	}
	return sanitizeVersion(line)
}

func sanitizeVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.ReplaceAll(version, string(filepath.Separator), "_")
	if len(version) > 120 {
		return version[:120]
	}
	return version
}
