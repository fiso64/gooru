package hashes

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestHashSourceMatchesFileStrategies(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{name: "small", data: bytes.Repeat([]byte("small-content-"), 100)},
		{name: "large", data: bytes.Repeat([]byte("large-content-0123456789"), 100000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "source.bin")
			if err := os.WriteFile(path, tc.data, 0600); err != nil {
				t.Fatal(err)
			}
			partialFile, err := HashFile(path)
			if err != nil {
				t.Fatal(err)
			}
			partialSource, err := HashSource(bytes.NewReader(tc.data), int64(len(tc.data)))
			if err != nil {
				t.Fatal(err)
			}
			if partialSource != partialFile {
				t.Fatalf("partial source hash = %s, file hash = %s", partialSource, partialFile)
			}
			fullFile, err := HashFileFull(path)
			if err != nil {
				t.Fatal(err)
			}
			fullSource, err := HashSourceFull(bytes.NewReader(tc.data), int64(len(tc.data)))
			if err != nil {
				t.Fatal(err)
			}
			if fullSource != fullFile {
				t.Fatalf("full source hash = %s, file hash = %s", fullSource, fullFile)
			}
		})
	}
}

func TestHashSourceRejectsInvalidSize(t *testing.T) {
	if _, err := HashSource(bytes.NewReader(nil), -1); err == nil {
		t.Fatal("partial source hash should reject negative size")
	}
	if _, err := HashSourceFull(bytes.NewReader(nil), -1); err == nil {
		t.Fatal("full source hash should reject negative size")
	}
}

func TestHashSourceRejectsTruncatedContent(t *testing.T) {
	data := []byte("short source")
	declaredSize := int64(len(data) + 1)
	if _, err := HashSourceFull(bytes.NewReader(data), declaredSize); err == nil {
		t.Fatal("full source hash should reject a source shorter than its declared size")
	}
	if _, err := HashSource(bytes.NewReader(data), declaredSize); err == nil {
		t.Fatal("partial source hash should reject a small source shorter than its declared size")
	}
}
