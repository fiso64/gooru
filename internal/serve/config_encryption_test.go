package serve

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gooru.local/internal/securekey"
)

func unsetEnvForTest(t *testing.T, name string) {
	t.Helper()
	old, ok := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(name, old)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}

func testEncryptionKey(fill byte) string {
	key := make([]byte, securekey.Size)
	for i := range key {
		key[i] = fill
	}
	return base64.StdEncoding.EncodeToString(key)
}

func TestLoadConfigResolvesEncryptionKey(t *testing.T) {
	unsetEnvForTest(t, securekey.EnvKeyFile)
	t.Setenv(securekey.EnvKey, testEncryptionKey(0x5a))
	configPath := filepath.Join(t.TempDir(), "serve.yaml")
	if err := os.WriteFile(configPath, []byte("encryption:\n  enabled: true\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configPath, filepath.Join(t.TempDir(), "gooru.db"), Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Encryption.Enabled || len(cfg.Encryption.Key) != securekey.Size || cfg.Encryption.Key[0] != 0x5a {
		t.Fatalf("encryption key was not resolved into runtime config")
	}
}

func TestLoadConfigEncryptionRequiresKey(t *testing.T) {
	unsetEnvForTest(t, securekey.EnvKey)
	unsetEnvForTest(t, securekey.EnvKeyFile)
	configPath := filepath.Join(t.TempDir(), "serve.yaml")
	if err := os.WriteFile(configPath, []byte("encryption:\n  enabled: true\n"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(configPath, filepath.Join(t.TempDir(), "gooru.db"), Overrides{})
	if err == nil || !strings.Contains(err.Error(), "requires an encryption key") {
		t.Fatalf("LoadConfig() error = %v, want missing encryption key error", err)
	}
}

func TestDefaultConfigLeavesEncryptionDisabled(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	if cfg.Encryption.Enabled || cfg.Encryption.Key != nil {
		t.Fatal("encryption should be disabled by default")
	}
}
