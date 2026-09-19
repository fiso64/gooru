package serve

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func tinyPDFDocument() []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 60 40] /Resources << >> /Contents 4 0 R >>",
		"<< /Length 0 >>\nstream\n\nendstream",
	}
	offsets := make([]int, 0, len(objects))
	for i, object := range objects {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	startXref := out.Len()
	out.WriteString("xref\n0 5\n0000000000 65535 f \n")
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", startXref)
	return out.Bytes()
}

func TestPDFThumbnailerRendersBoundedFirstPageWhenAvailable(t *testing.T) {
	path, err := exec.LookPath("pdftoppm")
	if err != nil { t.Skip("optional PDF renderer is not installed") }
	pdf := tinyPDFDocument()
	renderer := pdfThumbnailer{path: path, version: "pdftoppm:test"}
	for _, format := range []string{"jpeg", "png"} {
		t.Run(format, func(t *testing.T) {
			var imageBytes bytes.Buffer
			if err := renderer.ThumbnailSource("document.pdf", bytes.NewReader(pdf), &imageBytes, 64, format); err != nil {
				t.Fatal(err)
			}
			imageConfig, decodedFormat, err := image.DecodeConfig(bytes.NewReader(imageBytes.Bytes()))
			if err != nil { t.Fatal(err) }
			if decodedFormat != format { t.Fatalf("format = %q, want %q", decodedFormat, format) }
			if imageConfig.Width < 1 || imageConfig.Height < 1 || imageConfig.Width > 64 || imageConfig.Height > 64 {
				t.Fatalf("unbounded thumbnail dimensions %dx%d", imageConfig.Width, imageConfig.Height)
			}
		})
	}
}

func TestPDFThumbnailerStreamsLogicalSourceAndRejectsMissingBackend(t *testing.T) {
	pdf := tinyPDFDocument()
	if err := (pdfThumbnailer{}).ThumbnailSource("private.pdf", bytes.NewReader(pdf), io.Discard, 32, "png"); err == nil {
		t.Fatal("missing renderer must reject PDF thumbnail")
	}
	if err := (pdfThumbnailer{path: "unused"}).ThumbnailSource("private.pdf", bytes.NewReader(pdf), io.Discard, 0, "png"); err == nil {
		t.Fatal("invalid size was accepted")
	}
	tmp := t.TempDir()
	output := filepath.Join(tmp, "rendered.png")
	if err := os.WriteFile(output, []byte("fake PNG thumbnail"), 0600); err != nil { t.Fatal(err) }
	rendererPath := filepath.Join(tmp, "pdftoppm-stub")
	script := "#!/bin/sh\nIFS= read -r header\n[ \"$header\" = '%PDF-1.4' ] || exit 3\ncat \"" + output + "\"\n"
	if err := os.WriteFile(rendererPath, []byte(script), 0700); err != nil { t.Fatal(err) }
	var rendered bytes.Buffer
	// Deliberately pass an unrelated logical filename: the renderer must use
	// the authenticated bytes, never try to open a plaintext pathname.
	if err := (pdfThumbnailer{path: rendererPath}).ThumbnailSource("private.pdf", bytes.NewReader(pdf), &rendered, 32, "png"); err != nil {
		t.Fatal(err)
	}
	if got := rendered.String(); !strings.Contains(got, "fake PNG thumbnail") {
		t.Fatalf("logical PDF rendering returned %q", got)
	}
}
