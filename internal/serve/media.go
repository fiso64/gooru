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
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "image/gif"

	"golang.org/x/image/draw"
	"gooru.local/types"
)

var ErrUnsupportedMedia = errors.New("unsupported media")

type Thumbnailer interface {
	Thumbnail(src string, dst io.Writer, size int, format string) error
	BackendVersion() string
}

type GoImageThumbnailer struct{}

func (GoImageThumbnailer) BackendVersion() string {
	return "go-image-v2"
}

func (GoImageThumbnailer) Thumbnail(src string, dst io.Writer, size int, format string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnsupportedMedia, err)
	}
	resized := scaleImage(img, size)
	switch format {
	case "jpeg":
		return jpeg.Encode(dst, resized, &jpeg.Options{Quality: 84})
	case "png":
		return png.Encode(dst, resized)
	default:
		return fmt.Errorf("%w: thumbnail format %q", ErrUnsupportedMedia, format)
	}
}

type MediaService struct {
	cfg         Config
	thumbnailer Thumbnailer
	cacheMu     sync.Mutex
	cacheLocks  map[string]*cacheLock
}

type cacheLock struct {
	mu   sync.Mutex
	refs int
}

func NewMediaService(cfg Config) *MediaService {
	return &MediaService{cfg: cfg, thumbnailer: NewMediaThumbnailer(cfg)}
}

func (m *MediaService) ServeContent(w http.ResponseWriter, r *http.Request, file types.FileInfo) {
	f, err := os.Open(file.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "file content not found", nil)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "not_found", "file content not found", nil)
		return
	}
	http.ServeContent(w, r, filepath.Base(file.Path), info.ModTime(), f)
}

func (m *MediaService) ServeDownload(w http.ResponseWriter, r *http.Request, file types.FileInfo) {
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(file.Path)))
	m.ServeContent(w, r, file)
}

func (m *MediaService) ServeDerivative(w http.ResponseWriter, r *http.Request, file types.FileInfo, kind string) {
	size, err := m.derivativeSize(r, kind)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	format := strings.ToLower(strings.TrimSpace(m.cfg.Media.ThumbnailFormat))
	if format == "" {
		format = "jpeg"
	}
	cachePath, err := m.cachePath(file, kind, size, format)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to prepare media cache", nil)
		return
	}
	if _, err := os.Stat(cachePath); err == nil {
		w.Header().Set("X-Gooru-Cache", "hit")
		m.serveCachedDerivative(w, r, cachePath, format)
		return
	}
	unlock := m.lockCachePath(cachePath)
	defer unlock()
	if _, err := os.Stat(cachePath); err == nil {
		w.Header().Set("X-Gooru-Cache", "hit")
		m.serveCachedDerivative(w, r, cachePath, format)
		return
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0700); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to prepare media cache", nil)
		return
	}
	tmp := cachePath + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to prepare media cache", nil)
		return
	}
	genErr := m.thumbnailer.Thumbnail(file.Path, out, size, format)
	closeErr := out.Close()
	if genErr != nil || closeErr != nil {
		_ = os.Remove(tmp)
		var unsupported *UnsupportedMediaError
		if errors.As(genErr, &unsupported) {
			writeError(w, http.StatusUnsupportedMediaType, "unsupported_media", "thumbnail generation is unavailable for this file", unsupported.Details())
			return
		}
		if errors.Is(genErr, ErrUnsupportedMedia) {
			writeError(w, http.StatusUnsupportedMediaType, "unsupported_media", "thumbnail generation is unavailable for this file", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to generate thumbnail", nil)
		return
	}
	if err := os.Rename(tmp, cachePath); err != nil {
		_ = os.Remove(tmp)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to store thumbnail", nil)
		return
	}
	w.Header().Set("X-Gooru-Cache", "miss")
	m.serveCachedDerivative(w, r, cachePath, format)
}

func (m *MediaService) lockCachePath(path string) func() {
	m.cacheMu.Lock()
	if m.cacheLocks == nil {
		m.cacheLocks = make(map[string]*cacheLock)
	}
	lock := m.cacheLocks[path]
	if lock == nil {
		lock = &cacheLock{}
		m.cacheLocks[path] = lock
	}
	lock.refs++
	m.cacheMu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		m.cacheMu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(m.cacheLocks, path)
		}
		m.cacheMu.Unlock()
	}
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

func (m *MediaService) cachePath(file types.FileInfo, kind string, size int, format string) (string, error) {
	root, err := m.cacheRoot()
	if err != nil {
		return "", err
	}
	key := strings.Join([]string{
		file.Hash,
		mediaKindForType(mediaTypeForPath(file.Path)),
		kind,
		strconv.Itoa(size),
		format,
		m.thumbnailer.BackendVersion(),
	}, "|")
	sum := sha256.Sum256([]byte(key))
	name := hex.EncodeToString(sum[:]) + "." + derivativeExtension(format)
	return filepath.Join(root, name[:2], name), nil
}

func (m *MediaService) cacheRoot() (string, error) {
	if strings.TrimSpace(m.cfg.Media.CacheDir) != "" {
		return m.cfg.Media.CacheDir, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "gooru", "media-cache"), nil
}

func (m *MediaService) serveCachedDerivative(w http.ResponseWriter, r *http.Request, path string, format string) {
	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "thumbnail not found", nil)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "thumbnail not found", nil)
		return
	}
	w.Header().Set("Content-Type", mimeForDerivative(format))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, info.Name(), info.ModTime().Truncate(time.Second), f)
}

func scaleImage(src image.Image, maxSize int) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 || maxSize <= 0 {
		return src
	}
	targetW, targetH := width, height
	if width >= height && width > maxSize {
		targetW = maxSize
		targetH = maxSize * height / width
	} else if height > width && height > maxSize {
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
