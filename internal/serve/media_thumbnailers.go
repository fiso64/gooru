package serve

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

func (t *MediaThumbnailer) ThumbnailQuality(src string, dst io.Writer, size int, format string, quality int) error {
	kind := mediaKindForType(mediaTypeForPath(src))
	if kind != "photo" && kind != "gif" {
		return t.Thumbnail(src, dst, size, format)
	}
	if qualityThumbnailer, ok := t.imagePrimary.(QualityThumbnailer); ok {
		if err := qualityThumbnailer.ThumbnailQuality(src, dst, size, format, quality); err == nil {
			return nil
		}
	}
	if qualityThumbnailer, ok := t.imageFallback.(QualityThumbnailer); ok {
		return qualityThumbnailer.ThumbnailQuality(src, dst, size, format, quality)
	}
	return &UnsupportedMediaError{Backend: "image", Reason: "no image thumbnail backend supports configured quality", Err: ErrUnsupportedMedia}
}

func (t *MediaThumbnailer) Thumbnail(src string, dst io.Writer, size int, format string) error {
	kind := mediaKindForType(mediaTypeForPath(src))
	switch kind {
	case "photo", "gif":
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

func (t *MediaThumbnailer) ThumbnailSourceQuality(name string, src io.ReadSeeker, dst io.Writer, size int, format string, quality int) error {
	kind := mediaKindForType(mediaTypeForPath(name))
	if kind != "photo" && kind != "gif" {
		return t.ThumbnailSource(name, src, dst, size, format)
	}
	if qualityThumbnailer, ok := t.imageFallback.(SourceQualityThumbnailer); ok {
		return qualityThumbnailer.ThumbnailSourceQuality(name, src, dst, size, format, quality)
	}
	return &UnsupportedMediaError{Backend: "image", Reason: "image backend cannot apply configured quality to logical media sources", Err: ErrUnsupportedMedia}
}

func (t *MediaThumbnailer) ThumbnailSource(name string, src io.ReadSeeker, dst io.Writer, size int, format string) error {
	kind := mediaKindForType(mediaTypeForPath(name))
	switch kind {
	case "photo", "gif":
		if sourceThumbnailer, ok := t.imageFallback.(SourceThumbnailer); ok {
			return sourceThumbnailer.ThumbnailSource(name, src, dst, size, format)
		}
		return &UnsupportedMediaError{Backend: "image", Reason: "image backend cannot read protected media sources", Err: ErrUnsupportedMedia}
	case "video":
		if sourceThumbnailer, ok := t.video.(SourceThumbnailer); ok {
			return sourceThumbnailer.ThumbnailSource(name, src, dst, size, format)
		}
		return &UnsupportedMediaError{Backend: "ffmpeg", Reason: "video backend cannot read protected media sources", Err: ErrUnsupportedMedia}
	default:
		return &UnsupportedMediaError{Backend: "media", Reason: "media kind " + kind + " is not thumbnailable", Err: ErrUnsupportedMedia}
	}
}

type commandThumbnailer struct {
	backend   string
	path      string
	version   string
	run       func(ctx context.Context, path string, src string, dst string, size int, format string) error
	sourceRun func(ctx context.Context, path string, src io.ReadSeeker, dst io.Writer, size int, format string) error
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

func (t commandThumbnailer) ThumbnailSource(_ string, src io.ReadSeeker, dst io.Writer, size int, format string) error {
	if t.path == "" || t.sourceRun == nil {
		return &UnsupportedMediaError{Backend: t.backend, Reason: "backend cannot read protected media sources", Err: ErrUnsupportedMedia}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := t.sourceRun(ctx, t.path, src, dst, size, format); err != nil {
		if errors.Is(err, ErrUnsupportedMedia) {
			return err
		}
		return &UnsupportedMediaError{Backend: t.backend, Reason: "backend failed to generate derivative", Err: err}
	}
	return nil
}

func NewFFmpegVideoThumbnailer(ffmpegPath string, ffprobePath string) Thumbnailer {
	ffprobePath = resolveCommandPath(ffprobePath)
	ffmpeg := newCommandThumbnailer("ffmpeg", ffmpegPath, []string{"-version"}, func(ctx context.Context, exe string, src string, dst string, size int, format string) error {
		offset, ok := videoThumbnailOffset(ctx, ffprobePath, src)
		if ok {
			if err := runFFmpegThumbnail(ctx, exe, ffmpegThumbnailArgs(src, dst, size, format, &offset), dst); err == nil {
				return nil
			}
		}
		fallback := time.Duration(0)
		return runFFmpegThumbnail(ctx, exe, ffmpegThumbnailArgs(src, dst, size, format, &fallback), dst)
	})
	ffmpeg.sourceRun = func(ctx context.Context, exe string, src io.ReadSeeker, dst io.Writer, size int, format string) error {
		offset, _ := videoThumbnailOffsetFromSource(ctx, ffprobePath, src)
		if err := runFFmpegThumbnailFromSource(ctx, exe, src, dst, size, format, &offset); err == nil {
			return nil
		}
		fallback := time.Duration(0)
		return runFFmpegThumbnailFromSource(ctx, exe, src, dst, size, format, &fallback)
	}
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

func runFFmpegThumbnail(ctx context.Context, exe string, args []string, dst string) error {
	if err := commandError(exec.CommandContext(ctx, exe, args...).Run()); err != nil {
		return err
	}
	info, err := os.Stat(dst)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return errors.New("ffmpeg produced an empty thumbnail")
	}
	return nil
}

func videoThumbnailOffsetFromSource(ctx context.Context, ffprobePath string, src io.ReadSeeker) (time.Duration, bool) {
	if strings.TrimSpace(ffprobePath) == "" {
		return 3 * time.Second, true
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return 0, false
	}
	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		"pipe:0",
	)
	cmd.Stdin = src
	out, err := cmd.Output()
	_, seekErr := src.Seek(0, io.SeekStart)
	if err != nil || seekErr != nil {
		return 3 * time.Second, true
	}
	return videoOffsetFromDurationOutput(out)
}

func runFFmpegThumbnailFromSource(ctx context.Context, exe string, src io.ReadSeeker, dst io.Writer, size int, format string, offset *time.Duration) error {
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, exe, ffmpegThumbnailSourceArgs(size, format, offset)...)
	cmd.Stdin = src
	var output bytes.Buffer
	cmd.Stdout = &output
	if err := commandError(cmd.Run()); err != nil {
		return err
	}
	if output.Len() == 0 {
		return errors.New("ffmpeg produced an empty thumbnail")
	}
	_, err := io.Copy(dst, &output)
	return err
}

func ffmpegThumbnailSourceArgs(size int, format string, offset *time.Duration) []string {
	vcodec := "mjpeg"
	if format == "png" {
		vcodec = "png"
	}
	scale := fmt.Sprintf("scale=if(gte(iw\\,ih)\\,min(%d\\,iw)\\,-2):if(gte(ih\\,iw)\\,min(%d\\,ih)\\,-2)", size, size)
	args := []string{"-v", "error", "-i", "pipe:0"}
	if offset != nil && *offset > 0 {
		args = append(args, "-ss", formatSeconds(*offset))
	}
	return append(args,
		"-frames:v", "1",
		"-vf", scale,
		"-f", "image2pipe",
		"-vcodec", vcodec,
		"pipe:1",
	)
}

func resolveCommandPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return path
	}
	return resolved
}

func videoThumbnailOffset(ctx context.Context, ffprobePath string, src string) (time.Duration, bool) {
	if strings.TrimSpace(ffprobePath) == "" {
		return 3 * time.Second, true
	}
	out, err := exec.CommandContext(ctx, ffprobePath,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		src,
	).Output()
	if err != nil {
		return 3 * time.Second, true
	}
	return videoOffsetFromDurationOutput(out)
}

func videoOffsetFromDurationOutput(out []byte) (time.Duration, bool) {
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || seconds <= 0 {
		return 3 * time.Second, true
	}
	offset := time.Duration(seconds * 0.10 * float64(time.Second))
	if offset < 500*time.Millisecond {
		offset = 500*time.Millisecond
	}
	if offset > 3*time.Second {
		offset = 3*time.Second
	}
	if max := time.Duration(seconds*float64(time.Second)) - 250*time.Millisecond; max > 0 && offset > max {
		offset = max
	}
	return offset, true
}

func ffmpegThumbnailArgs(src string, dst string, size int, format string, offset *time.Duration) []string {
	vcodec := "mjpeg"
	if format == "png" {
		vcodec = "png"
	}
	scale := fmt.Sprintf("scale=if(gte(iw\\,ih)\\,min(%d\\,iw)\\,-2):if(gte(ih\\,iw)\\,min(%d\\,ih)\\,-2)", size, size)
	args := []string{"-v", "error", "-y"}
	if offset != nil && *offset > 0 {
		args = append(args, "-ss", formatSeconds(*offset))
	}
	args = append(args,
		"-i", src,
		"-frames:v", "1",
		"-vf", scale,
		"-f", "image2",
		"-vcodec", vcodec,
		dst,
	)
	return args
}

func formatSeconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
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

var commandVersionCache = struct {
	sync.Mutex
	values map[string]string
}{values: make(map[string]string)}

func commandVersion(path string, args []string) string {
	key := path + "\x00" + strings.Join(args, "\x00")
	commandVersionCache.Lock()
	defer commandVersionCache.Unlock()
	if version, ok := commandVersionCache.values[key]; ok {
		return version
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
	version := "unknown"
	if err == nil {
		line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
		if line != "" {
			version = sanitizeVersion(line)
		}
	}
	commandVersionCache.values[key] = version
	return version
}

func sanitizeVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.ReplaceAll(version, string(filepath.Separator), "_")
	if len(version) > 120 {
		return version[:120]
	}
	return version
}
