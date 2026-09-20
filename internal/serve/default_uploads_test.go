package serve

import "testing"

func TestDefaultUploadPathResolves(t *testing.T) {
	if path, err := defaultUploadPath(); err != nil || path == "" {
		t.Fatalf("%q: %v", path, err)
	}
}
