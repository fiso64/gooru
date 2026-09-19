package serve

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gooru.local/types"
)

const (
	pdfViewerPageEdge = 1200
	pdfViewerMaxPages = 10000
	pdfInfoMaxBytes = 64 << 10
)

type pdfViewerService struct {
	renderer pdfThumbnailer
	infoPath string
}

type pdfDocumentManifest struct {
	PageCount int `json:"page_count"`
	PageURLPrefix string `json:"page_url_prefix"`
}

func newPDFViewerService() *pdfViewerService {
	renderer := newPDFThumbnailer().(pdfThumbnailer)
	infoPath, _ := exec.LookPath("pdfinfo")
	return &pdfViewerService{renderer: renderer, infoPath: infoPath}
}

func (m *MediaService) ServePDF(w http.ResponseWriter, r *http.Request, file types.FileInfo, publicID string) {
	if mediaKindForType(mediaTypeForPath(file.Path)) != "pdf" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media", "file is not a PDF", nil)
		return
	}
	if m.pdfViewer == nil || m.pdfViewer.renderer.path == "" || m.pdfViewer.infoPath == "" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media", "PDF viewing requires Poppler pdfinfo and pdftoppm", nil)
		return
	}

	rawPage, pageRequested := r.URL.Query()["page"]
	if pageRequested {
		if len(rawPage) != 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "page must be a single integer", nil)
			return
		}
		page, err := strconv.Atoi(rawPage[0])
		if err != nil || page < 1 || page > pdfViewerMaxPages {
			writeError(w, http.StatusBadRequest, "invalid_request", "page must be between 1 and 10000", nil)
			return
		}
		m.servePDFPage(w, r, file, page)
		return
	}

	source, err := m.openMediaSource(fileStoragePath(file))
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "PDF source is unavailable", nil)
		return
	}
	defer source.Close()

	pageCount, err := m.pdfViewer.pageCount(r.Context(), source)
	if err != nil {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media", "PDF page count cannot be read", nil)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, pdfDocumentManifest{
		PageCount: pageCount,
		PageURLPrefix: "/api/v1/files/" + publicID + "/pdf?page=",
	})
}

func (v *pdfViewerService) pageCount(requestContext context.Context, src io.ReadSeeker) (int, error) {
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(requestContext, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, v.infoPath, "-")
	cmd.Stdin = src
	var stdout bytes.Buffer
	bounded := &pdfLimitedWriter{Writer: &stdout, limit: pdfInfoMaxBytes}
	cmd.Stdout = bounded
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("PDF metadata command failed: %w", err)
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		if !strings.HasPrefix(line, "Pages:") {
			continue
		}
		count, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Pages:")))
		if err == nil && count > 0 && count <= pdfViewerMaxPages {
			return count, nil
		}
		return 0, errors.New("PDF page count is invalid or exceeds viewer limit")
	}
	return 0, errors.New("PDF page count is missing")
}

func (m *MediaService) servePDFPage(w http.ResponseWriter, r *http.Request, file types.FileInfo, page int) {
	if m.derivativeStoreErr != nil || m.derivatives == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to prepare media cache", nil)
		return
	}
	cacheKey := fmt.Sprintf("%s|pdf-view-v1|%s|%d|%d", file.Hash, m.pdfViewer.renderer.version, page, pdfViewerPageEdge)
	sum := sha256.Sum256([]byte(cacheKey))
	name := hex.EncodeToString(sum[:]) + ".png"
	artifact, err := m.derivatives.GetOrGenerate(filepath.Join(name[:2], name), func(dst io.Writer) error {
		source, err := m.openMediaSource(fileStoragePath(file))
		if err != nil {
			return err
		}
		defer source.Close()
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		return m.pdfViewer.renderer.renderPage(ctx, source, dst, page, pdfViewerPageEdge, "png", pdfThumbnailMaxBytes)
	})
	if err != nil {
		m.writeThumbnailGenerationError(w, err)
		return
	}
	defer artifact.Close()
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Gooru-Cache", artifact.CacheStatus)
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", artifact.CacheControl)
	http.ServeContent(w, r, artifact.Name, artifact.ModTime, artifact.Reader)
}
