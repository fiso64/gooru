package serve

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommandVersionCachesResult(t *testing.T) {
	dir := t.TempDir()
	commandPath := filepath.Join(dir, "version-command")
	countPath := filepath.Join(dir, "count")
	script := "#!/bin/sh\nprintf x >> \"" + countPath + "\"\nprintf 'fake-tool 1.0\\n'\n"
	if err := os.WriteFile(commandPath, []byte(script), 0700); err != nil {
		t.Fatalf("write fake version command: %v", err)
	}

	if got := commandVersion(commandPath, []string{"-version"}); got != "fake-tool 1.0" {
		t.Fatalf("first version = %q, want fake-tool 1.0", got)
	}
	if got := commandVersion(commandPath, []string{"-version"}); got != "fake-tool 1.0" {
		t.Fatalf("second version = %q, want fake-tool 1.0", got)
	}
	count, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatalf("read invocation count: %v", err)
	}
	if got := len(count); got != 1 {
		t.Fatalf("version command ran %d times, want 1", got)
	}
}
