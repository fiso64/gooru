package serve

import (
	"archive/zip"
	"encoding/json"
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
	"sort"
	"strconv"
	"strings"

	_ "golang.org/x/image/webp"
	"gooru.local/types"
)

const (
	maxComicPageBytes      int64 = 96 << 20
	maxCachedComicArchives       = 4
)

var errNotComicArchive = errors.New("not a comic archive")

type comicPage struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	URL   string `json:"url"`
}

type comicManifest struct {
	Pages []comicPage `json:"pages"`
}

type comicArchive struct {
	reader *zip.Reader
	pages  []*zip.File
	close  func() error
}

type cachedComicArchive struct {
	key      string
	path     string
	archive  *comicArchive
	refs     int
	lastUsed uint64
}

func openComicArchive(path string) (*comicArchive, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open cbz: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat cbz: %w", err)
	}
	archive, err := openComicArchiveReader(path, file, info.Size(), file.Close)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return archive, nil
}

func (m *MediaService) openComicArchive(path string) (*comicArchive, error) {
	source, err := m.openMediaSource(path)
	if err != nil {
		return nil, fmt.Errorf("open cbz: %w", err)
	}
	archive, err := openComicArchiveReader(path, source, source.size, source.Close)
	if err != nil {
		_ = source.Close()
		return nil, err
	}
	return archive, nil
}

func comicArchiveCacheKey(path string, size, modTimeNano int64) string {
	return path + "\x00" + strconv.FormatInt(size, 10) + "\x00" + strconv.FormatInt(modTimeNano, 10)
}

func (m *MediaService) acquireComicArchive(file types.FileInfo) (*comicArchive, func(), error) {
	// Open the current source before consulting the cache. This makes cache
	// invalidation follow the file actually being served rather than possibly
	// stale database metadata, and avoids a stat/open race on cache misses.
	source, err := m.openMediaSource(fileStoragePath(file))
	if err != nil {
		return nil, nil, fmt.Errorf("open cbz: %w", err)
	}
	key := comicArchiveCacheKey(file.Path, source.size, source.modTime.UnixNano())

	m.comicMu.Lock()
	if entry := m.comicCache[key]; entry != nil {
		m.comicTick++
		entry.refs++
		entry.lastUsed = m.comicTick
		m.comicMu.Unlock()
		_ = source.Close()
		return entry.archive, func() { m.releaseComicArchive(entry) }, nil
	}
	m.comicMu.Unlock()

	archive, err := openComicArchiveReader(file.Path, source, source.size, source.Close)
	if err != nil {
		_ = source.Close()
		return nil, nil, err
	}
	entry := &cachedComicArchive{key: key, path: file.Path, archive: archive, refs: 1}

	m.comicMu.Lock()
	if m.comicCache == nil {
		m.comicCache = make(map[string]*cachedComicArchive)
	}
	if existing := m.comicCache[key]; existing != nil {
		m.comicTick++
		existing.refs++
		existing.lastUsed = m.comicTick
		m.comicMu.Unlock()
		_ = archive.Close()
		return existing.archive, func() { m.releaseComicArchive(existing) }, nil
	}
	m.comicTick++
	entry.lastUsed = m.comicTick
	m.comicCache[key] = entry
	stale := m.removeIdleComicArchivesForPathLocked(file.Path, key)
	evicted := m.evictIdleComicArchivesLocked()
	m.comicMu.Unlock()
	closeComicArchives(append(stale, evicted...))

	return archive, func() { m.releaseComicArchive(entry) }, nil
}

func (m *MediaService) releaseComicArchive(entry *cachedComicArchive) {
	var closed []*comicArchive
	m.comicMu.Lock()
	if entry.refs > 0 {
		entry.refs--
	}
	if entry.refs == 0 && m.comicCache[entry.key] == entry {
		for _, other := range m.comicCache {
			if other.path == entry.path && other.key != entry.key {
				delete(m.comicCache, entry.key)
				closed = append(closed, entry.archive)
				break
			}
		}
	}
	closed = append(closed, m.evictIdleComicArchivesLocked()...)
	m.comicMu.Unlock()
	closeComicArchives(closed)
}

func (m *MediaService) removeIdleComicArchivesForPathLocked(path, keepKey string) []*comicArchive {
	var removed []*comicArchive
	for key, entry := range m.comicCache {
		if key == keepKey || entry.path != path || entry.refs != 0 {
			continue
		}
		delete(m.comicCache, key)
		removed = append(removed, entry.archive)
	}
	return removed
}

func (m *MediaService) evictIdleComicArchivesLocked() []*comicArchive {
	var removed []*comicArchive
	for len(m.comicCache) > maxCachedComicArchives {
		var oldest *cachedComicArchive
		for _, entry := range m.comicCache {
			if entry.refs != 0 || (oldest != nil && entry.lastUsed >= oldest.lastUsed) {
				continue
			}
			oldest = entry
		}
		if oldest == nil {
			break
		}
		delete(m.comicCache, oldest.key)
		removed = append(removed, oldest.archive)
	}
	return removed
}

func closeComicArchives(archives []*comicArchive) {
	for _, archive := range archives {
		_ = archive.Close()
	}
}

// openComicArchiveReader contains the archive logic independently of the
// filesystem. The caller retains ownership of readerAt if construction fails;
// on success comicArchive owns closeFn. Encrypted media can later provide
// authenticated random-access plaintext here without changing page bounds,
// ordering, or HTTP behavior.
func openComicArchiveReader(name string, readerAt io.ReaderAt, size int64, closeFn func() error) (*comicArchive, error) {
	if !strings.EqualFold(filepath.Ext(name), ".cbz") {
		return nil, errNotComicArchive
	}
	reader, err := zip.NewReader(readerAt, size)
	if err != nil {
		return nil, fmt.Errorf("open cbz: %w", err)
	}
	pages := make([]*zip.File, 0, len(reader.File))
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || !isComicImageName(file.Name) {
			continue
		}
		pages = append(pages, file)
	}
	sort.SliceStable(pages, func(i, j int) bool { return naturalLess(pages[i].Name, pages[j].Name) })
	if len(pages) == 0 {
		return nil, fmt.Errorf("%w: archive has no supported image pages", ErrUnsupportedMedia)
	}
	return &comicArchive{reader: reader, pages: pages, close: closeFn}, nil
}

func (a *comicArchive) Close() error {
	if a.close == nil {
		return nil
	}
	return a.close()
}

func (a *comicArchive) openPage(index int) (io.ReadCloser, *zip.File, error) {
	if index < 0 || index >= len(a.pages) {
		return nil, nil, ErrNotFound
	}
	page := a.pages[index]
	if page.UncompressedSize64 > uint64(maxComicPageBytes) {
		return nil, nil, fmt.Errorf("%w: comic page exceeds %d bytes", ErrUnsupportedMedia, maxComicPageBytes)
	}
	reader, err := page.Open()
	if err != nil {
		return nil, nil, err
	}
	return reader, page, nil
}

func isComicImageName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return true
	default:
		return false
	}
}

func naturalLess(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for len(a) > 0 && len(b) > 0 {
		if isASCIIDigit(a[0]) && isASCIIDigit(b[0]) {
			ai, bi := numericPrefixLength(a), numericPrefixLength(b)
			an := strings.TrimLeft(a[:ai], "0")
			bn := strings.TrimLeft(b[:bi], "0")
			if an == "" {
				an = "0"
			}
			if bn == "" {
				bn = "0"
			}
			if len(an) != len(bn) {
				return len(an) < len(bn)
			}
			if an != bn {
				return an < bn
			}
			a, b = a[ai:], b[bi:]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func isASCIIDigit(value byte) bool { return value >= '0' && value <= '9' }

func numericPrefixLength(value string) int {
	index := 0
	for index < len(value) && isASCIIDigit(value[index]) {
		index++
	}
	return index
}

func (m *MediaService) ServeComic(w http.ResponseWriter, r *http.Request, file types.FileInfo, publicID string) {
	archive, release, err := m.acquireComicArchive(file)
	if errors.Is(err, errNotComicArchive) || errors.Is(err, ErrUnsupportedMedia) {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media", "file is not a readable CBZ archive", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to open comic archive", nil)
		return
	}
	defer release()

	rawPage := strings.TrimSpace(r.URL.Query().Get("page"))
	if rawPage == "" {
		pages := make([]comicPage, 0, len(archive.pages))
		for index, page := range archive.pages {
			pages = append(pages, comicPage{
				Index: index,
				Name:  filepath.Base(page.Name),
				URL:   "/api/v1/comics/" + publicID + "/" + strconv.Itoa(index),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(comicManifest{Pages: pages})
		return
	}

	index, err := strconv.Atoi(rawPage)
	if err != nil || index < 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "page must be a non-negative integer", nil)
		return
	}
	reader, page, err := archive.openPage(index)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "comic page not found", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media", "comic page cannot be served", nil)
		return
	}
	defer reader.Close()

	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(page.Name)))
	if semicolon := strings.IndexByte(contentType, ';'); semicolon >= 0 {
		contentType = contentType[:semicolon]
	}
	if contentType == "" || !strings.HasPrefix(contentType, "image/") {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Ordinary mode opts comic pages into a short private cache. Protected API
	// middleware owns the stronger no-store policy and enforces it at commit,
	// so this feature handler does not need to know whether protection is active.
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = io.Copy(w, io.LimitReader(reader, maxComicPageBytes))
}

func (m *MediaService) thumbnailCBZFirstPage(src string, dst io.Writer, size int, format string) error {
	archive, err := m.openComicArchive(src)
	if err != nil {
		return err
	}
	defer archive.Close()
	return thumbnailComicArchiveFirstPage(archive, dst, size, format)
}

func thumbnailCBZFirstPage(src string, dst io.Writer, size int, format string) error {
	archive, err := openComicArchive(src)
	if err != nil {
		return err
	}
	defer archive.Close()
	return thumbnailComicArchiveFirstPage(archive, dst, size, format)
}

func thumbnailComicArchiveFirstPage(archive *comicArchive, dst io.Writer, size int, format string) error {
	reader, _, err := archive.openPage(0)
	if err != nil {
		return err
	}
	defer reader.Close()
	img, _, err := image.Decode(io.LimitReader(reader, maxComicPageBytes))
	if err != nil {
		return fmt.Errorf("%w: decode first comic page: %v", ErrUnsupportedMedia, err)
	}
	resized := scaleImage(img, size)
	switch format {
	case "jpeg":
		return jpeg.Encode(dst, resized, &jpeg.Options{Quality: derivativeJPEGQuality})
	case "png":
		return png.Encode(dst, resized)
	default:
		return fmt.Errorf("%w: thumbnail format %q", ErrUnsupportedMedia, format)
	}
}
