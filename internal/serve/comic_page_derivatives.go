package serve

import (
    "archive/zip"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "io"
    "net/http"
    "path/filepath"
    "strconv"
    "strings"

    "gooru.local/types"
)

// Each archive entry is a logical image source. Acquisition enforces the CBZ
// page byte limit; transformation and cache storage are shared with standalone
// media rather than extracting a temporary plaintext file.
func comicPreviewEligible(page *zip.File) bool {
    if page.UncompressedSize64 > uint64(maxComicPageBytes) {
        return false
    }
    switch strings.ToLower(filepath.Ext(page.Name)) {
    case ".jpg", ".jpeg", ".png", ".webp":
        return true
    default:
        // Preserve animation for GIF pages as in standalone preview mode.
        return false
    }
}

func (m *MediaService) comicPageLosslessAvailable(file types.FileInfo, archive *comicArchive, index int) bool {
    if !m.cfg.Media.TranscodeCBZPages || !m.cfg.Media.LosslessJPEGTranscode ||
        m.losslessJPEGTool == "" || file.Hash == "" || index < 0 || index >= len(archive.pages) {
        return false
    }
    ext := strings.ToLower(filepath.Ext(archive.pages[index].Name))
    if ext != ".jpg" && ext != ".jpeg" {
        return false
    }
    src, _, err := archive.openPage(index)
    if err != nil {
        return false
    }
    defer src.Close()
    progressive, err := jpegFrameIsProgressive(io.LimitReader(src, maxComicPageBytes))
    return err == nil && progressive
}

func (m *MediaService) comicPageDerivativePath(file types.FileInfo, archive *comicArchive, index int, variant string, size int, format string) string {
    page := archive.pages[index]
    quality := ""
    if variant == "preview" && format == "jpeg" {
        quality = strconv.Itoa(m.cfg.Media.PreviewJPEGQuality)
    }
    // The archive cache key is based on the actual logical source's path,
    // size and modification time, not stale library metadata. Page identity,
    // CRC and dimensions additionally distinguish entries within each source.
    key := strings.Join([]string{
        "comic-page-v1", file.Hash, archive.version, strconv.Itoa(index),
        page.Name, strconv.FormatUint(uint64(page.CRC32), 10),
        strconv.FormatUint(page.UncompressedSize64, 10),
        strconv.FormatUint(page.CompressedSize64, 10),
        variant, strconv.Itoa(size), format, quality,
        m.thumbnailer.BackendVersion(), m.losslessJPEGTool,
    }, "|")
    sum := sha256.Sum256([]byte(key))
    name := hex.EncodeToString(sum[:]) + "." + derivativeExtension(format)
    return filepath.Join(name[:2], name)
}

func (m *MediaService) serveComicPageDerivative(w http.ResponseWriter, r *http.Request, file types.FileInfo, archive *comicArchive, index int, variant string) {
    if index < 0 || index >= len(archive.pages) {
        writeError(w, http.StatusNotFound, "not_found", "comic page not found", nil)
        return
    }
    if !m.cfg.Media.TranscodeCBZPages {
        writeError(w, http.StatusNotFound, "not_found", "comic page derivative is unavailable", nil)
        return
    }

    size, format := 0, "jpeg"
    switch variant {
    case "preview":
        if !m.cfg.Media.PreviewEnabled || !comicPreviewEligible(archive.pages[index]) {
            writeError(w, http.StatusNotFound, "not_found", "comic page preview is unavailable", nil)
            return
        }
        size = m.cfg.Media.PreviewSize
        format = strings.ToLower(strings.TrimSpace(m.cfg.Media.ThumbnailFormat))
        if format == "" {
            format = "jpeg"
        }
    case "lossless":
        if !m.comicPageLosslessAvailable(file, archive, index) {
            writeError(w, http.StatusNotFound, "not_found", "comic page lossless derivative is unavailable", nil)
            return
        }
    default:
        writeError(w, http.StatusBadRequest, "invalid_request", "unsupported comic page variant", nil)
        return
    }

    if m.derivativeStoreErr != nil || m.derivatives == nil {
        writeError(w, http.StatusInternalServerError, "internal_error", "failed to prepare media cache", nil)
        return
    }
    relative := m.comicPageDerivativePath(file, archive, index, variant, size, format)
    artifact, err := m.derivatives.GetOrGenerate(relative, func(dst io.Writer) error {
        src, _, err := archive.openPage(index)
        if err != nil {
            return err
        }
        defer src.Close()
        bounded := io.LimitReader(src, maxComicPageBytes)
        if variant == "lossless" {
            return transcodeProgressiveJPEG(r.Context(), bounded, dst, m.losslessJPEGTool)
        }
        return encodeResizedImage(bounded, dst, size, format, m.cfg.Media.PreviewJPEGQuality)
    })
    if err != nil {
        m.writeThumbnailGenerationError(w, fmt.Errorf("comic page %d %s: %w", index, variant, err))
        return
    }
    defer artifact.Close()
    w.Header().Set("X-Gooru-Cache", artifact.CacheStatus)
    w.Header().Set("X-Content-Type-Options", "nosniff")
    w.Header().Set("Content-Type", mimeForDerivative(format))
    w.Header().Set("Cache-Control", artifact.CacheControl)
    if artifact.CacheControl == "private, no-store" {
        w.Header().Set("Pragma", "no-cache")
        w.Header().Set("Expires", "0")
    }
    http.ServeContent(w, r, artifact.Name, artifact.ModTime, artifact.Reader)
}
