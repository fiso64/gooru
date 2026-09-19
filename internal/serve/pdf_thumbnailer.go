package serve

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// PDF thumbnails use a bounded external renderer. The logical source is
// streamed directly to stdin, including in protected mode.
type pdfThumbnailer struct {
	path string
	version string
}

func newPDFThumbnailer() Thumbnailer {
	path, err := exec.LookPath("pdftoppm")
	if err != nil {
		return pdfThumbnailer{version: "pdftoppm:missing"}
	}
	return pdfThumbnailer{path: path, version: "pdftoppm:" + commandVersion(path, []string{"-v"})}
}

func (t pdfThumbnailer) BackendVersion() string { return t.version }

func (t pdfThumbnailer) Thumbnail(path string, dst io.Writer, size int, format string) error {
	src, err := os.Open(path)
	if err != nil { return err }
	defer src.Close()
	return t.ThumbnailSource(path, src, dst, size, format)
}

func (t pdfThumbnailer) ThumbnailSource(_ string, src io.ReadSeeker, dst io.Writer, size int, format string) error {
	if t.path == "" {
		return &UnsupportedMediaError{Backend: "pdftoppm", Reason: "PDF renderer is unavailable", Err: ErrUnsupportedMedia}
	}
	if size < 1 || size > 4096 { return fmt.Errorf("%w: invalid PDF thumbnail size", ErrUnsupportedMedia) }
	if format != "jpeg" && format != "png" { return fmt.Errorf("%w: invalid PDF thumbnail format", ErrUnsupportedMedia) }
	if _, err := src.Seek(0, io.SeekStart); err != nil { return err }
	args := []string{"-f", "1", "-l", "1", "-singlefile", "-scale-to", strconv.Itoa(size)}
	if format == "jpeg" { args = append(args, "-jpeg") } else { args = append(args, "-png") }
	args = append(args, "-")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, t.path, args...)
	cmd.Stdin = src
	cmd.Stdout = dst
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return &UnsupportedMediaError{Backend: "pdftoppm", Reason: "first-page rendering failed", Err: err}
	}
	return nil
}
