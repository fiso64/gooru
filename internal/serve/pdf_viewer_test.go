package serve

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPDFViewerCountsPagesThroughLogicalSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pdfinfo")
	script := "#!/bin/sh\nIFS= read -r header\n[ \"$header\" = '%PDF-1.4' ] || exit 3\nprintf 'Pages: 2\\n'\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil { t.Fatal(err) }
	pages, err := (&pdfViewerService{infoPath: path}).pageCount(context.Background(), bytes.NewReader(tinyPDFDocument()))
	if err != nil { t.Fatal(err) }
	if pages != 2 { t.Fatalf("pages = %d, want 2", pages) }
}
