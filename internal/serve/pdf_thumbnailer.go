package serve

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"
)

const (
	pdfProbeEdge = 128
	pdfProbeMaxBytes = 4 << 20
	pdfThumbnailMaxBytes = 32 << 20
	pdfThumbnailMaxEdge = 8192
)

var errPDFOutputLimit = errors.New("PDF renderer output exceeds thumbnail byte limit")

// PDF thumbnails stream the logical source directly to the renderer's stdin,
// including in protected mode; no plaintext temporary source file is created.
type pdfThumbnailer struct {
	path    string
	version string
}

func newPDFThumbnailer() Thumbnailer {
	path, err := exec.LookPath("pdftoppm")
	if err != nil {
		return pdfThumbnailer{version: "pdftoppm:missing|short-edge-v2"}
	}
	return pdfThumbnailer{path: path, version: "pdftoppm:" + commandVersion(path, []string{"-v"}) + "|short-edge-v2"}
}

func (t pdfThumbnailer) BackendVersion() string { return t.version }

func (t pdfThumbnailer) Thumbnail(path string, dst io.Writer, size int, format string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	return t.ThumbnailSource(path, src, dst, size, format)
}

func (t pdfThumbnailer) ThumbnailSource(_ string, src io.ReadSeeker, dst io.Writer, size int, format string) error {
	if t.path == "" {
		return &UnsupportedMediaError{Backend: "pdftoppm", Reason: "PDF renderer is unavailable", Err: ErrUnsupportedMedia}
	}
	if size < 1 || size > 4096 {
		return fmt.Errorf("%w: invalid PDF thumbnail size", ErrUnsupportedMedia)
	}
	if format != "jpeg" && format != "png" {
		return fmt.Errorf("%w: invalid PDF thumbnail format", ErrUnsupportedMedia)
	}

	// Poppler's -scale-to controls the LONG edge. Probe the first page at a
	// small size before selecting the long-edge limit that preserves Gooru's
	// short-edge thumbnail contract for both landscape and portrait pages.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var probe bytes.Buffer
	if err := t.render(ctx, src, &probe, pdfProbeEdge, "png", pdfProbeMaxBytes); err != nil {
		return err
	}
	dimensions, _, err := image.DecodeConfig(bytes.NewReader(probe.Bytes()))
	if err != nil || dimensions.Width < 1 || dimensions.Height < 1 {
		return &UnsupportedMediaError{Backend: "pdftoppm", Reason: "PDF renderer returned an invalid first-page preview", Err: ErrUnsupportedMedia}
	}
	shortEdge, longEdge := dimensions.Width, dimensions.Height
	if shortEdge > longEdge {
		shortEdge, longEdge = longEdge, shortEdge
	}
	// Round to the nearest output pixel; avoid enormous rasterizations for
	// pathologically elongated PDF pages.
	targetLongEdge := (size*longEdge + shortEdge/2) / shortEdge
	if targetLongEdge > pdfThumbnailMaxEdge {
		return &UnsupportedMediaError{Backend: "pdftoppm", Reason: "PDF first-page aspect ratio exceeds thumbnail raster limit", Err: ErrUnsupportedMedia}
	}
	if targetLongEdge < size {
		targetLongEdge = size
	}
	return t.render(ctx, src, dst, targetLongEdge, format, pdfThumbnailMaxBytes)
}

func (t pdfThumbnailer) render(ctx context.Context, src io.ReadSeeker, dst io.Writer, longEdge int, format string, byteLimit int64) error {
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return err
	}
	args := []string{"-f", "1", "-l", "1", "-singlefile", "-scale-to", strconv.Itoa(longEdge)}
	if format == "jpeg" {
		args = append(args, "-jpeg")
	} else {
		args = append(args, "-png")
	}
	args = append(args, "-")
	cmd := exec.CommandContext(ctx, t.path, args...)
	cmd.Stdin = src
	output := &pdfLimitedWriter{Writer: dst, limit: byteLimit}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if output.exceeded {
			return &UnsupportedMediaError{Backend: "pdftoppm", Reason: "PDF renderer output exceeded thumbnail byte limit", Err: errPDFOutputLimit}
		}
		return &UnsupportedMediaError{Backend: "pdftoppm", Reason: "first-page rendering failed", Err: err}
	}
	if output.written == 0 {
		return &UnsupportedMediaError{Backend: "pdftoppm", Reason: "PDF renderer produced no image", Err: ErrUnsupportedMedia}
	}
	return nil
}

type pdfLimitedWriter struct {
	io.Writer
	written  int64
	limit    int64
	exceeded bool
}

func (w *pdfLimitedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.limit-w.written {
		w.exceeded = true
		return 0, errPDFOutputLimit
	}
	n, err := w.Writer.Write(p)
	w.written += int64(n)
	return n, err
}
