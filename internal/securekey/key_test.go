package securekey

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func encodedKey(fill byte) string {
	key := make([]byte, Size)
	for i := range key {
		key[i] = fill
	}
	return base64.StdEncoding.EncodeToString(key)
}

func TestLoadProcessIsOptIn(t *testing.T) {
	t.Setenv(EnvKey, "")
	if err := os.Unsetenv(EnvKey); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvKeyFile, "")
	key, enabled, err := LoadProcess()
	if err != nil || enabled || key != nil {
		t.Fatalf("disabled process key = (%v, %v, %v), want nil, false, nil", key, enabled, err)
	}

	t.Setenv(EnvKey, encodedKey(0x44))
	key, enabled, err = LoadProcess()
	if err != nil || !enabled || len(key) != Size || key[0] != 0x44 {
		t.Fatalf("enabled process key did not resolve: enabled=%v len=%d err=%v", enabled, len(key), err)
	}
}

func TestLoadProcessRejectsCompetingSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, []byte(encodedKey(0x11)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvKey, encodedKey(0x22))
	t.Setenv(EnvKeyFile, path)
	if _, enabled, err := LoadProcess(); !enabled || err == nil {
		t.Fatalf("expected enabled process configuration with competing sources to fail")
	}
}

func TestLoadFromEnvironment(t *testing.T) {
	const name = "GOORU_TEST_ENCRYPTION_KEY"
	t.Setenv(name, "  "+encodedKey(0x2a)+"\n")
	key, err := Load(Source{Env: name})
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != Size || key[0] != 0x2a || key[Size-1] != 0x2a {
		t.Fatalf("unexpected decoded key")
	}
}

func TestLoadFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, []byte(encodedKey(0x7f)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	key, err := Load(Source{File: path})
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != Size || key[0] != 0x7f {
		t.Fatalf("unexpected decoded key")
	}
}

func TestLoadRequiresExactlyOneSource(t *testing.T) {
	if _, err := Load(Source{}); err == nil {
		t.Fatal("expected missing source to fail")
	}
	if _, err := Load(Source{Env: "A", File: "B"}); err == nil {
		t.Fatal("expected multiple sources to fail")
	}
}

func TestLoadRejectsMissingOrMalformedKey(t *testing.T) {
	const missing = "GOORU_TEST_MISSING_ENCRYPTION_KEY"
	if _, err := Load(Source{Env: missing}); err == nil {
		t.Fatal("expected missing environment variable to fail")
	}

	const malformed = "GOORU_TEST_BAD_ENCRYPTION_KEY"
	t.Setenv(malformed, "not-base64")
	if _, err := Load(Source{Env: malformed}); err == nil {
		t.Fatal("expected malformed base64 to fail")
	}

	const short = "GOORU_TEST_SHORT_ENCRYPTION_KEY"
	t.Setenv(short, base64.StdEncoding.EncodeToString([]byte("short")))
	if _, err := Load(Source{Env: short}); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey, got %v", err)
	}
}

func TestLoadRejectsExposedKeyFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not authoritative on Windows")
	}
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, []byte(encodedKey(0x31)), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(Source{File: path}); err == nil {
		t.Fatal("expected group/world-readable key file to fail")
	}
}
