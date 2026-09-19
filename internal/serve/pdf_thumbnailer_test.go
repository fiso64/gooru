package serve

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gooru.local/types"
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

func testPDFPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := png.Encode(&output, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestPDFThumbnailerRendersFirstPageAtShortEdgeWhenAvailable(t *testing.T) {
	path, err := exec.LookPath("pdftoppm")
	if err != nil { t.Skip("optional PDF renderer not installed") }
	for _, format := range []string{"jpeg", "png"} {
		t.Run(format, func(t *testing.T) {
			var output bytes.Buffer
			if err := (pdfThumbnailer{path: path}).ThumbnailSource("document.pdf", bytes.NewReader(tinyPDFDocument()), &output, 64, format); err != nil { t.Fatal(err) }
			dimensions, actual, err := image.DecodeConfig(bytes.NewReader(output.Bytes()))
			if err != nil { t.Fatal(err) }
			if actual != format { t.Fatalf("format = %q, want %q", actual, format) }
			short, long := dimensions.Width, dimensions.Height
			if short > long { short, long = long, short }
			if short < 63 || short > 65 || long > 128 { t.Fatalf("short-edge thumbnail 64: got %dx%d", dimensions.Width, dimensions.Height) }
		})
	}
}

func TestPDFThumbnailerStreamsSourceAndSelectsShortEdge(t *testing.T) {
	for _, dimensions := range []image.Point{{X: 96, Y: 64}, {X: 64, Y: 96}} {
		t.Run(fmt.Sprint(dimensions), func(t *testing.T) {
			dir := t.TempDir()
			picture := testPDFPNG(t, dimensions.X, dimensions.Y)
			picturePath := filepath.Join(dir, "raster")
			if err := os.WriteFile(picturePath, picture, 0600); err != nil { t.Fatal(err) }
			argsPath := filepath.Join(dir, "args")
			rendererPath := filepath.Join(dir, "renderer")
			script := "#!/bin/sh\nIFS= read -r header\n[ \"$header\" = '%PDF-1.4' ] || exit 3\nprintf '%s\\n' \"$*\" >> \"" + argsPath + "\"\ncat \"" + picturePath + "\"\n"
			if err := os.WriteFile(rendererPath, []byte(script), 0700); err != nil { t.Fatal(err) }
			var output bytes.Buffer
			if err := (pdfThumbnailer{path: rendererPath}).ThumbnailSource("private.pdf", bytes.NewReader(tinyPDFDocument()), &output, 64, "png"); err != nil { t.Fatal(err) }
			if !bytes.Equal(output.Bytes(), picture) { t.Fatal("renderer output changed") }
			args, err := os.ReadFile(argsPath)
			if err != nil { t.Fatal(err) }
			if !strings.Contains(string(args), "-scale-to 128") || !strings.Contains(string(args), "-scale-to 96") { t.Fatalf("missing short-edge conversion: %q", args) }
		})
	}
}

func TestPDFThumbnailerRejectsUnavailableAndEmptyRenderer(t *testing.T) {
	pdf := bytes.NewReader(tinyPDFDocument())
	if err := (pdfThumbnailer{}).ThumbnailSource("private.pdf", pdf, io.Discard, 32, "png"); err == nil { t.Fatal("missing renderer must fail") }
	if err := (pdfThumbnailer{path: "unused"}).ThumbnailSource("private.pdf", pdf, io.Discard, 0, "png"); err == nil { t.Fatal("invalid size accepted") }
	path := filepath.Join(t.TempDir(), "silent-renderer")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil { t.Fatal(err) }
	if err := (pdfThumbnailer{path: path}).ThumbnailSource("document.pdf", pdf, io.Discard, 16, "png"); err == nil { t.Fatal("empty renderer output must not be cached") }
}

func TestPDFThumbnailerBoundsOutput(t *testing.T) {
	var output bytes.Buffer
	limited := &pdfLimitedWriter{Writer: &output, limit: 5}
	if _, err := limited.Write([]byte("12345")); err != nil { t.Fatal(err) }
	if _, err := limited.Write([]byte("6")); !errors.Is(err, errPDFOutputLimit) { t.Fatalf("output limit error = %v", err) }
	if !limited.exceeded || output.Len() != 5 { t.Fatalf("bounded writer state = %+v, size = %d", limited, output.Len()) }
}

func TestProtectedPDFThumbnailUsesLogicalEncryptedSource(t *testing.T) {
	pdf := tinyPDFDocument()
	sourcePath := writeNamedMediaFile(t, "private.pdf", pdf)
	file := types.FileInfo{ID: 190, Path: sourcePath, Hash: "protected-pdf-thumb", Size: int64(len(pdf))}
	server := protectedMediaTestServer(t, file)
	encryptMediaFixture(t, sourcePath, server.cfg.Encryption.Key)
	rendered := testPDFPNG(t, 96, 64)
	outputFile := filepath.Join(t.TempDir(), "image")
	if err := os.WriteFile(outputFile, rendered, 0600); err != nil { t.Fatal(err) }
	rendererPath := filepath.Join(t.TempDir(), "pdf-renderer")
	script := "#!/bin/sh\nIFS= read -r header\n[ \"$header\" = '%PDF-1.4' ] || exit 3\ncat \"" + outputFile + "\"\n"
	if err := os.WriteFile(rendererPath, []byte(script), 0700); err != nil { t.Fatal(err) }
	server.media.thumbnailer.(*MediaThumbnailer).pdf = pdfThumbnailer{path: rendererPath, version: "test"}
	for index, wantCache := range []string{"miss", "hit"} {
		rec := httptest.NewRecorder()
		req := authedRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(file.ID)+"/thumbnail?size=16")
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK { t.Fatalf("request %d status %d: %s", index, rec.Code, rec.Body.String()) }
		if got := rec.Header().Get("X-Gooru-Cache"); got != wantCache { t.Fatalf("request %d cache = %q, want %q", index, got, wantCache) }
		if !bytes.Equal(rec.Body.Bytes(), rendered) { t.Fatalf("request %d logical source output differs", index) }
		if got := rec.Header().Get("Cache-Control"); got != "private, no-store" { t.Fatalf("protected PDF cache-control = %q", got) }
	}
}
