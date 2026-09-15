package serve

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	_ "image/gif"

	"golang.org/x/image/draw"
	"gooru.local/internal/filesource"
	"gooru.local/types"
)

var ErrUnsupportedMedia = errors.New("unsupported media")

const derivativeJPEGQuality = 92

type Thumbnailer interface {
	Thumbnail(src string, dst io.Writer, size int, format string) error
	BackendVersion() string
}

type SourceThumbnailer interface {
	ThumbnailSource(name string, src io.ReadSeeker, dst io.Writer, size int, format string) error
}

type QualityThumbnailer interface {
	ThumbnailQuality(src string, dst io.Writer, size int, format string, quality int) error
}

type SourceQualityThumbnailer interface {
	ThumbnailSourceQuality(name string, src io.ReadSeeker, dst io.Writer, size int, format string, quality int) error
}

type GoImageThumbnailer struct{}

func (GoImageThumbnailer) BackendVersion() string {
	return "go-image-v4"
}

func (t GoImageThumbnailer) Thumbnail(src string, dst io.Writer, size int, format string) error {
	return t.ThumbnailQuality(src, dst, size, format, derivativeJPEGQuality)
}

func (t GoImageThumbnailer) ThumbnailQuality(src string, dst io.Writer, size int, format string, quality int) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()
	return t.ThumbnailSourceQuality(src, file, dst, size, format, quality)
}

func (t GoImageThumbnailer) ThumbnailSource(name string, src io.ReadSeeker, dst io.Writer, size int, format string) error {
	return t.ThumbnailSourceQuality(name, src, dst, size, format, derivativeJPEGQuality)
}

func (GoImageThumbnailer) ThumbnailSourceQuality(name string, src io.ReadSeeker, dst io.Writer, size int, format string, quality int) error {
	if err := thumbnailImageSourcePrimary(name, src, dst, size, format, quality); err == nil {
		return nil
	} else if !errors.Is(err, ErrUnsupportedMedia) {
		return err
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return err
	}
	img, _, err := image.Decode(src)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnsupportedMedia, err)
	}
	resized := scaleImage(img, size)
	switch format {
	case "jpeg":
		return jpeg.Encode(dst, resized, &jpeg.Options{Quality: quality})
	case "png":
		return png.Encode(dst, resized)
	default:
		return fmt.Errorf("%w: thumbnail format %q", ErrUnsupportedMedia, format)
	}
}

type MediaService struct {
	cfg                        Config
	thumbnailer                Thumbnailer
	thumbnailGeneration        thumbnailGenerationPolicy
	thumbnailQualityGeneration thumbnailQualityGenerationPolicy
	sourceResolver             *filesource.Resolver
	sourceResolverErr          error
	derivatives                derivativeStore
	derivativeStoreErr         error
	comicMu                    sync.Mutex
	comicCache                 map[string]*cachedComicArchive
	comicTick                  uint64
}

func NewMediaService(cfg Config) *MediaService {
	return newComposedMediaServiceFromConfig(cfg)
}

func newMediaSourceResolver(cfg Config) (*filesource.Resolver, error) {
	if !cfg.Encryption.Enabled {
		return filesource.NewFilesystem(), nil
	}
	roots := make([]string, 0, len(cfg.Uploads.Targets))
	for _, target := range cfg.Uploads.Targets {
		roots = append(roots, target.Path)
	}
	return filesource.NewProtected(cfg.Encryption.Key, roots)
}

func newDerivativeStore(cfg Config) (derivativeStore, error) {
	root, err := derivativeCacheRoot(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.Encryption.Enabled {
		return newEncryptedDerivativeStore(root, cfg.Encryption.Key), nil
	}
	return newPersistentDerivativeStore(root), nil
}

func derivativeCacheRoot(cfg Config) (string, error) {
	if strings.TrimSpace(cfg.Media.CacheDir) != "" {
		return cfg.Media.CacheDir, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "gooru", "media-cache"), nil
}

func (m *MediaService) ServeContent(w http.ResponseWriter, r *http.Request, file types.FileInfo) {
	m.serveOriginal(w, r, file, false)
}

func (m *MediaService) ServeDownload(w http.ResponseWriter, r *http.Request, file types.FileInfo) {
	m.serveOriginal(w, r, file, true)
}

func applyOriginalContentPolicy(w http.ResponseWriter, file types.FileInfo) {
	contentType := originalContentType(file)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if isInlineOriginalMedia(contentType) {
		w.Header().Set("Content-Type", contentType)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(file.Path)))
}

func originalContentType(file types.FileInfo) string {
	if file.Metadata != nil && strings.TrimSpace(file.Metadata.MimeType) != "" {
		return strings.ToLower(strings.TrimSpace(file.Metadata.MimeType))
	}
	if byExt := mime.TypeByExtension(strings.ToLower(filepath.Ext(file.Path))); byExt != "" {
		if semicolon := strings.IndexByte(byExt, ';'); semicolon >= 0 {
			byExt = byExt[:semicolon]
		}
		return strings.ToLower(strings.TrimSpace(byExt))
	}
	return "application/octet-stream"
}

func isInlineOriginalMedia(contentType string) bool {
	switch contentType {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/avif",
		"video/mp4", "video/webm", "video/ogg",
		"audio/mpeg", "audio/mp4", "audio/ogg", "audio/wav", "audio/webm", "audio/flac":
		return true
	default:
		return false
	}
}

func (m *MediaService) ServeDerivative(w http.ResponseWriter, r *http.Request, file types.FileInfo, kind string) {
	if kind == "preview" && !m.cfg.Media.PreviewEnabled {
		m.ServeContent(w, r, file)
		return
	}
	if kind == "preview" && mediaKindForType(originalContentType(file)) == "gif" {
		m.ServeContent(w, r, file)
		return
	}

	size, err := m.derivativeSize(r, kind)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	format := strings.ToLower(strings.TrimSpace(m.cfg.Media.ThumbnailFormat))
	if format == "" {
		format = "jpeg"
	}
	if m.derivativeStoreErr != nil || m.derivatives == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to prepare media cache", nil)
		return
	}
	relativePath := m.derivativeRelativePath(file, kind, size, format)
	artifact, err := m.derivatives.GetOrGenerate(relativePath, func(dst io.Writer) error {
		return m.generateDerivative(file, dst, size, format, kind)
	})
	if err != nil {
		m.writeThumbnailGenerationError(w, err)
		return
	}
	defer artifact.Close()
	w.Header().Set("X-Gooru-Cache", artifact.CacheStatus)
	w.Header().Set("Content-Type", mimeForDerivative(format))
	w.Header().Set("Cache-Control", artifact.CacheControl)
	if artifact.CacheControl == "private, no-store" {
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
	}
	http.ServeContent(w, r, artifact.Name, artifact.ModTime, artifact.Reader)
}

func (m *MediaService) writeThumbnailGenerationError(w http.ResponseWriter, err error) {
	var unsupported *UnsupportedMediaError
	if errors.As(err, &unsupported) {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media", "thumbnail generation is unavailable for this file", unsupported.Details())
		return
	}
	if errors.Is(err, ErrUnsupportedMedia) {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media", "thumbnail generation is unavailable for this file", nil)
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", "failed to generate thumbnail", nil)
}

func (m *MediaService) generateDerivative(file types.FileInfo, dst io.Writer, size int, format string, kind string) error {
	if kind == "preview" && format == "jpeg" && m.cfg.Media.PreviewJPEGQuality != derivativeJPEGQuality {
		mediaKind := mediaKindForType(mediaTypeForPath(file.Path))
		if (mediaKind == "photo" || mediaKind == "gif") && m.thumbnailQualityGeneration != nil {
			handled, err := m.thumbnailQualityGeneration(m, file, dst, size, format, m.cfg.Media.PreviewJPEGQuality)
			if handled {
				return err
			}
		}
	}
	return m.generateThumbnail(file, dst, size, format)
}

func (m *MediaService) generateThumbnail(file types.FileInfo, dst io.Writer, size int, format string) error {
	if strings.EqualFold(filepath.Ext(file.Path), ".cbz") {
		return m.thumbnailCBZFirstPageFile(file, dst, size, format)
	}
	if m.thumbnailGeneration == nil {
		return errors.New("thumbnail generation policy is not configured")
	}
	return m.thumbnailGeneration(m, file, dst, size, format)
}

func (m *MediaService) derivativeSize(r *http.Request, kind string) (int, error) {
	if kind == "preview" {
		return m.cfg.Media.PreviewSize, nil
	}
	raw := r.URL.Query().Get("size")
	if raw == "" {
		if len(m.cfg.Media.ThumbnailSizes) == 0 {
			return 0, fmt.Errorf("no thumbnail sizes are configured")
		}
		return m.cfg.Media.ThumbnailSizes[0], nil
	}
	size, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("size must be an allowed integer")
	}
	for _, allowed := range m.cfg.Media.ThumbnailSizes {
		if size == allowed {
			return size, nil
		}
	}
	return 0, fmt.Errorf("size must be one of: %s", intList(m.cfg.Media.ThumbnailSizes))
}

func (m *MediaService) derivativeRelativePath(file types.FileInfo, kind string, size int, format string) string {
	backendVersion := m.thumbnailer.BackendVersion()
	if strings.EqualFold(filepath.Ext(file.Path), ".cbz") {
		backendVersion += "|cbz-cover-v1"
	}
	qualityKey := ""
	if kind == "preview" && format == "jpeg" {
		qualityKey = strconv.Itoa(m.cfg.Media.PreviewJPEGQuality)
	}
	key := strings.Join([]string{
		file.Hash,
		mediaKindForType(mediaTypeForPath(file.Path)),
		kind,
		strconv.Itoa(size),
		format,
		qualityKey,
		backendVersion,
	}, "|")
	sum := sha256.Sum256([]byte(key))
	name := hex.EncodeToString(sum[:]) + "." + derivativeExtension(format)
	return filepath.Join(name[:2], name)
}

func scaleImage(src image.Image, maxSize int) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 || maxSize <= 0 {
		return src
	}
	targetW, targetH := width, height
	if width <= height && width > maxSize {
		targetW = maxSize
		targetH = maxSize * height / width
	} else if height < width && height > maxSize {
		targetH = maxSize
		targetW = maxSize * width / height
	}
	if targetW <= 0 {
		targetW = 1
	}
	if targetH <= 0 {
		targetH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	return dst
}

func derivativeExtension(format string) string {
	if format == "jpeg" {
		return "jpg"
	}
	return format
}

func mimeForDerivative(format string) string {
	if format == "png" {
		return "image/png"
	}
	return "image/jpeg"
}

func intList(values []int) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, ", ")
}
