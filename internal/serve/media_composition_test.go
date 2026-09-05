package serve

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestServerMediaRuntimeConfigDoesNotRetainEncryptionKey(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x42}, 32)
	cfg.Uploads.Targets = []UploadTarget{{ID: "managed", Path: t.TempDir()}}

	server := NewServerWithLibrary(cfg, nil)
	if len(server.media.cfg.Encryption.Key) != 0 {
		t.Fatal("media runtime config retained raw encryption key material")
	}
	if !server.media.cfg.Encryption.Enabled {
		t.Fatal("media runtime config lost protected-mode behavior flag")
	}
	if server.media.sourceResolver == nil {
		t.Fatal("media service did not receive composed logical source resolver")
	}
}
