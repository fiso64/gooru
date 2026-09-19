package serve

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"gooru.local/types"
)

func isOriginalDocumentNavigation(r *http.Request) bool {
	destination := strings.TrimSpace(r.Header.Get("Sec-Fetch-Dest"))
	if destination != "" {
		return strings.EqualFold(destination, "document") || strings.EqualFold(destination, "iframe")
	}

	// Fetch Metadata is widely supported, but fall back to the normal browser
	// navigation Accept header so older clients can still inspect originals.
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/html")
}

func applyOriginalDocumentContentPolicy(w http.ResponseWriter, file types.FileInfo, source io.ReaderAt) {
	declaredType := originalContentType(file)
	detectedType := detectOriginalContentType(source)

	w.Header().Set("X-Content-Type-Options", "nosniff")
	if contentType := inlineOriginalDocumentType(declaredType, detectedType); contentType != "" {
		setOriginalDocumentInlineHeaders(w, file, contentType)
		return
	}
	if isInspectableOriginalText(file, declaredType, detectedType) {
		// Scriptable originals such as HTML, SVG, and JavaScript are useful to inspect,
		// but must not execute with the application's origin. Plain text plus nosniff
		// keeps the tab useful without turning user files into same-origin active content.
		setOriginalDocumentInlineHeaders(w, file, "text/plain; charset=utf-8")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	if w.Header().Get("Content-Disposition") == "" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(file.Path)))
	}
}

func setOriginalDocumentInlineHeaders(w http.ResponseWriter, file types.FileInfo, contentType string) {
	w.Header().Set("Content-Type", contentType)
	if contentType == "application/pdf" {
		// Only PDFs may be framed by the same-origin document viewer.
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'; base-uri 'self'; form-action 'self'")
	}
	// ServeDownload installs attachment before delegating to ServeContent. Never
	// overwrite it, otherwise the explicit download action would stop downloading.
	if w.Header().Get("Content-Disposition") == "" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filepath.Base(file.Path)))
	}
}

func inlineOriginalDocumentType(declaredType, detectedType string) string {
	for _, contentType := range []string{declaredType, detectedType} {
		contentType = normalizeOriginalMediaType(contentType)
		if contentType == "application/pdf" || isInlineOriginalMedia(contentType) {
			return contentType
		}
	}
	return ""
}

func isInspectableOriginalText(file types.FileInfo, declaredType, detectedType string) bool {
	candidates := []string{declaredType, detectedType}
	if extensionType := mime.TypeByExtension(strings.ToLower(filepath.Ext(file.Path))); extensionType != "" {
		candidates = append(candidates, extensionType)
	}
	for _, candidate := range candidates {
		contentType := normalizeOriginalMediaType(candidate)
		if strings.HasPrefix(contentType, "text/") {
			return true
		}
		switch contentType {
		case "application/json", "application/xml", "application/yaml", "application/x-yaml",
			"application/toml", "application/javascript", "application/x-javascript",
			"application/ecmascript":
			return true
		}
		if strings.HasSuffix(contentType, "+json") || strings.HasSuffix(contentType, "+xml") {
			return true
		}
	}
	return false
}

func detectOriginalContentType(source io.ReaderAt) string {
	var buf [512]byte
	n, err := source.ReadAt(buf[:], 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return ""
	}
	if n == 0 {
		return ""
	}
	return normalizeOriginalMediaType(http.DetectContentType(buf[:n]))
}

func normalizeOriginalMediaType(contentType string) string {
	contentType = strings.TrimSpace(strings.ToLower(contentType))
	if semicolon := strings.IndexByte(contentType, ';'); semicolon >= 0 {
		contentType = contentType[:semicolon]
	}
	return strings.TrimSpace(contentType)
}
