package serve

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestNewMediaServiceDoesNotRetainEncryptionKey(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = append([]byte(nil), key...)
	cfg.Uploads.Targets = []UploadTarget{{ID: "managed", Path: t.TempDir()}}

	media := NewMediaService(cfg)
	if len(media.cfg.Encryption.Key) != 0 {
		t.Fatal("media runtime config retained raw encryption key material")
	}
	if !media.cfg.Encryption.Enabled {
		t.Fatal("media runtime config lost protected-mode behavior flag")
	}
	if media.sourceResolver == nil {
		t.Fatal("media service did not receive composed logical source resolver")
	}
	if !bytes.Equal(cfg.Encryption.Key, key) {
		t.Fatal("media construction mutated the caller encryption key")
	}
}
