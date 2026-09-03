from pathlib import Path


def replace(path, old, new):
    p = Path(path)
    text = p.read_text()
    if old not in text:
        raise SystemExit(f"expected text not found in {path}: {old!r}")
    p.write_text(text.replace(old, new, 1))


replace(
    "internal/serve/media.go",
    '''type Thumbnailer interface {\n\tThumbnail(src string, dst io.Writer, size int, format string) error\n\tBackendVersion() string\n}\n''',
    '''type Thumbnailer interface {\n\tThumbnail(src string, dst io.Writer, size int, format string) error\n\tBackendVersion() string\n}\n\ntype SourceThumbnailer interface {\n\tThumbnailSource(name string, src io.ReadSeeker, dst io.Writer, size int, format string) error\n}\n''',
)
replace(
    "internal/serve/media.go",
    '''func (GoImageThumbnailer) Thumbnail(src string, dst io.Writer, size int, format string) error {\n\tfile, err := os.Open(src)\n\tif err != nil {\n\t\treturn err\n\t}\n\tdefer file.Close()\n\timg, _, err := image.Decode(file)\n''',
    '''func (t GoImageThumbnailer) Thumbnail(src string, dst io.Writer, size int, format string) error {\n\tfile, err := os.Open(src)\n\tif err != nil {\n\t\treturn err\n\t}\n\tdefer file.Close()\n\treturn t.ThumbnailSource(src, file, dst, size, format)\n}\n\nfunc (GoImageThumbnailer) ThumbnailSource(_ string, src io.ReadSeeker, dst io.Writer, size int, format string) error {\n\tif _, err := src.Seek(0, io.SeekStart); err != nil {\n\t\treturn err\n\t}\n\timg, _, err := image.Decode(src)\n''',
)
replace(
    "internal/serve/media.go",
    '''func (m *MediaService) generateThumbnail(file types.FileInfo, dst io.Writer, size int, format string) error {\n\tif strings.EqualFold(filepath.Ext(file.Path), ".cbz") {\n\t\treturn m.thumbnailCBZFirstPage(file.Path, dst, size, format)\n\t}\n\treturn m.thumbnailer.Thumbnail(file.Path, dst, size, format)\n}\n''',
    '''func (m *MediaService) generateThumbnail(file types.FileInfo, dst io.Writer, size int, format string) error {\n\tif strings.EqualFold(filepath.Ext(file.Path), ".cbz") {\n\t\treturn m.thumbnailCBZFirstPage(file.Path, dst, size, format)\n\t}\n\tif m.cfg.Encryption.Enabled {\n\t\tsource, err := m.openMediaSource(file.Path)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tdefer source.Close()\n\t\tsourceThumbnailer, ok := m.thumbnailer.(SourceThumbnailer)\n\t\tif !ok {\n\t\t\treturn &UnsupportedMediaError{Backend: "media", Reason: "thumbnail backend cannot read protected media sources", Err: ErrUnsupportedMedia}\n\t\t}\n\t\treturn sourceThumbnailer.ThumbnailSource(file.Path, source, dst, size, format)\n\t}\n\treturn m.thumbnailer.Thumbnail(file.Path, dst, size, format)\n}\n''',
)

replace(
    "internal/serve/media_thumbnailers.go",
    '''func (t *MediaThumbnailer) Thumbnail(src string, dst io.Writer, size int, format string) error {\n\tkind := mediaKindForType(mediaTypeForPath(src))\n''',
    '''func (t *MediaThumbnailer) Thumbnail(src string, dst io.Writer, size int, format string) error {\n\tkind := mediaKindForType(mediaTypeForPath(src))\n''',
)
# Insert source dispatch after Thumbnail method.
p = Path("internal/serve/media_thumbnailers.go")
text = p.read_text()
needle = '''\tdefault:\n\t\treturn &UnsupportedMediaError{Backend: "media", Reason: "media kind " + kind + " is not thumbnailable", Err: ErrUnsupportedMedia}\n\t}\n}\n\ntype commandThumbnailer struct {\n'''
replacement = '''\tdefault:\n\t\treturn &UnsupportedMediaError{Backend: "media", Reason: "media kind " + kind + " is not thumbnailable", Err: ErrUnsupportedMedia}\n\t}\n}\n\nfunc (t *MediaThumbnailer) ThumbnailSource(name string, src io.ReadSeeker, dst io.Writer, size int, format string) error {\n\tkind := mediaKindForType(mediaTypeForPath(name))\n\tswitch kind {\n\tcase "photo", "gif":\n\t\tif sourceThumbnailer, ok := t.imageFallback.(SourceThumbnailer); ok {\n\t\t\treturn sourceThumbnailer.ThumbnailSource(name, src, dst, size, format)\n\t\t}\n\t\treturn &UnsupportedMediaError{Backend: "image", Reason: "image backend cannot read protected media sources", Err: ErrUnsupportedMedia}\n\tcase "video":\n\t\tif sourceThumbnailer, ok := t.video.(SourceThumbnailer); ok {\n\t\t\treturn sourceThumbnailer.ThumbnailSource(name, src, dst, size, format)\n\t\t}\n\t\treturn &UnsupportedMediaError{Backend: "ffmpeg", Reason: "video backend cannot read protected media sources", Err: ErrUnsupportedMedia}\n\tdefault:\n\t\treturn &UnsupportedMediaError{Backend: "media", Reason: "media kind " + kind + " is not thumbnailable", Err: ErrUnsupportedMedia}\n\t}\n}\n\ntype commandThumbnailer struct {\n'''
if needle not in text:
    raise SystemExit("commandThumbnailer insertion point missing")
text = text.replace(needle, replacement, 1)
p.write_text(text)

replace(
    "internal/serve/media_thumbnailers.go",
    '''type commandThumbnailer struct {\n\tbackend string\n\tpath    string\n\tversion string\n\trun     func(ctx context.Context, path string, src string, dst string, size int, format string) error\n}\n''',
    '''type commandThumbnailer struct {\n\tbackend   string\n\tpath      string\n\tversion   string\n\trun       func(ctx context.Context, path string, src string, dst string, size int, format string) error\n\tsourceRun func(ctx context.Context, path string, src io.ReadSeeker, dst io.Writer, size int, format string) error\n}\n''',
)
# Insert command source method before NewFFmpegVideoThumbnailer.
p = Path("internal/serve/media_thumbnailers.go")
text = p.read_text()
needle = '''func NewFFmpegVideoThumbnailer(ffmpegPath string, ffprobePath string) Thumbnailer {\n'''
method = '''func (t commandThumbnailer) ThumbnailSource(_ string, src io.ReadSeeker, dst io.Writer, size int, format string) error {\n\tif t.path == "" || t.sourceRun == nil {\n\t\treturn &UnsupportedMediaError{Backend: t.backend, Reason: "backend cannot read protected media sources", Err: ErrUnsupportedMedia}\n\t}\n\tctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)\n\tdefer cancel()\n\tif err := t.sourceRun(ctx, t.path, src, dst, size, format); err != nil {\n\t\tif errors.Is(err, ErrUnsupportedMedia) {\n\t\t\treturn err\n\t\t}\n\t\treturn &UnsupportedMediaError{Backend: t.backend, Reason: "backend failed to generate derivative", Err: err}\n\t}\n\treturn nil\n}\n\n'''
if needle not in text:
    raise SystemExit("ffmpeg insertion point missing")
text = text.replace(needle, method + needle, 1)
p.write_text(text)

# Add sourceRun setup before version handling.
replace(
    "internal/serve/media_thumbnailers.go",
    '''\tffprobeVersion := commandThumbnailer{backend: "ffprobe", path: strings.TrimSpace(ffprobePath), version: "ffprobe:missing"}\n''',
    '''\tffmpeg.sourceRun = func(ctx context.Context, exe string, src io.ReadSeeker, dst io.Writer, size int, format string) error {\n\t\toffset, _ := videoThumbnailOffsetFromSource(ctx, ffprobePath, src)\n\t\tif err := runFFmpegThumbnailFromSource(ctx, exe, src, dst, size, format, &offset); err == nil {\n\t\t\treturn nil\n\t\t}\n\t\tfallback := time.Duration(0)\n\t\treturn runFFmpegThumbnailFromSource(ctx, exe, src, dst, size, format, &fallback)\n\t}\n\tffprobeVersion := commandThumbnailer{backend: "ffprobe", path: strings.TrimSpace(ffprobePath), version: "ffprobe:missing"}\n''',
)

# Insert source helpers before resolveCommandPath.
p = Path("internal/serve/media_thumbnailers.go")
text = p.read_text()
needle = '''func resolveCommandPath(path string) string {\n'''
helpers = r'''func videoThumbnailOffsetFromSource(ctx context.Context, ffprobePath string, src io.ReadSeeker) (time.Duration, bool) {
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
	counter := &countingWriter{Writer: dst}
	cmd.Stdout = counter
	if err := commandError(cmd.Run()); err != nil {
		return err
	}
	if counter.n == 0 {
		return errors.New("ffmpeg produced an empty thumbnail")
	}
	return nil
}

type countingWriter struct {
	io.Writer
	n int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.n += int64(n)
	return n, err
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

'''
if needle not in text:
    raise SystemExit("source helper insertion point missing")
text = text.replace(needle, helpers + needle, 1)
p.write_text(text)

# Factor duration parsing so file/source probing use identical policy.
replace(
    "internal/serve/media_thumbnailers.go",
    '''\tseconds, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)\n\tif err != nil || seconds <= 0 {\n\t\treturn 3 * time.Second, true\n\t}\n\toffset := time.Duration(seconds * 0.10 * float64(time.Second))\n''',
    '''\treturn videoOffsetFromDurationOutput(out)\n}\n\nfunc videoOffsetFromDurationOutput(out []byte) (time.Duration, bool) {\n\tseconds, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)\n\tif err != nil || seconds <= 0 {\n\t\treturn 3 * time.Second, true\n\t}\n\toffset := time.Duration(seconds * 0.10 * float64(time.Second))\n''',
)

# Extend protected source tests.
p = Path("internal/serve/media_source_encryption_test.go")
text = p.read_text()
text = text.replace('''import (\n\t"bytes"\n\t"image/color"\n''', '''import (\n\t"bytes"\n\t"image"\n\t"image/color"\n''', 1)
text = text.replace('''\t"os"\n\t"path/filepath"\n''', '''\t"os"\n\t"os/exec"\n\t"path/filepath"\n''', 1)
text += r'''

func TestProtectedImageThumbnailReadsEncryptedOriginal(t *testing.T) {
	plaintext := tinyPNG(t, 32, 24, color.RGBA{R: 200, G: 40, B: 90, A: 255})
	path := writeNamedMediaFile(t, "secret.png", plaintext)
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x35}, securekey.Size)
	cfg.Media.CacheDir = filepath.Join(t.TempDir(), "cache")
	cfg.Media.ThumbnailSizes = []int{16}
	cfg.Media.ThumbnailFormat = "png"
	encryptMediaFixture(t, path, cfg.Encryption.Key)

	service := NewMediaService(cfg)
	recorder := httptest.NewRecorder()
	service.ServeDerivative(recorder, httptest.NewRequest(http.MethodGet, "/thumbnail?size=16", nil), types.FileInfo{Path: path, Hash: "protected-image"}, "thumbnail")
	if recorder.Code != http.StatusOK {
		t.Fatalf("protected image thumbnail status = %d: %s", recorder.Code, recorder.Body.String())
	}
	decoded, _, err := image.Decode(bytes.NewReader(recorder.Body.Bytes()))
	if err != nil {
		t.Fatalf("decode protected image thumbnail: %v", err)
	}
	if got := decoded.Bounds().Dx(); got != 16 {
		t.Fatalf("thumbnail width = %d, want 16", got)
	}
	if got := recorder.Header().Get("X-Gooru-Cache"); got != "bypass" {
		t.Fatalf("protected image thumbnail cache = %q, want bypass", got)
	}
	if _, err := os.Stat(cfg.Media.CacheDir); !os.IsNotExist(err) {
		t.Fatalf("protected image thumbnail must not create plaintext cache, stat error = %v", err)
	}
}

func TestProtectedVideoThumbnailStreamsDecryptedSource(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	path := writeTestVideo(t, ffmpeg, 32, 24)
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x61}, securekey.Size)
	cfg.Tools.FFmpegPath = ffmpeg
	cfg.Tools.FFprobePath = ffprobe
	cfg.Media.CacheDir = filepath.Join(t.TempDir(), "cache")
	cfg.Media.ThumbnailSizes = []int{16}
	cfg.Media.ThumbnailFormat = "png"
	encryptMediaFixture(t, path, cfg.Encryption.Key)

	service := NewMediaService(cfg)
	recorder := httptest.NewRecorder()
	service.ServeDerivative(recorder, httptest.NewRequest(http.MethodGet, "/thumbnail?size=16", nil), types.FileInfo{Path: path, Hash: "protected-video"}, "thumbnail")
	if recorder.Code != http.StatusOK {
		t.Fatalf("protected video thumbnail status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if _, _, err := image.Decode(bytes.NewReader(recorder.Body.Bytes())); err != nil {
		t.Fatalf("decode protected video thumbnail: %v", err)
	}
	if got := recorder.Header().Get("X-Gooru-Cache"); got != "bypass" {
		t.Fatalf("protected video thumbnail cache = %q, want bypass", got)
	}
	if _, err := os.Stat(cfg.Media.CacheDir); !os.IsNotExist(err) {
		t.Fatalf("protected video thumbnail must not create plaintext cache, stat error = %v", err)
	}
}
'''
p.write_text(text)
