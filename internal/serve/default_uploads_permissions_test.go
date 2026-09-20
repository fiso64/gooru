package serve

import (
	"os"
	"testing"
)

func TestDefaultUploadDirectoryPrivate(t *testing.T) {
	testDefaultUploadRoot(t)
	path, err := defaultUploadPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := prepareDefaultUploadDir(path); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{path} {
		info, err := os.Lstat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() || info.Mode().Perm() != 0700 {
			t.Fatalf("insecure path %s: %v", dir, info.Mode())
		}
	}
}
