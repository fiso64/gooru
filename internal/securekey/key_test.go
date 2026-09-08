package securekey

import (
	"bytes"
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

func TestDeriveBindsSubkeysToPurpose(t *testing.T) {
	master := bytes.Repeat([]byte{0x42}, Size)
	first, err := Derive(master, "gooru/test/first")
	if err != nil {
		t.Fatal(err)
	}
	again, err := Derive(master, "gooru/test/first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Derive(master, "gooru/test/second")
	if err != nil {
		t.Fatal(err)
	}
	if first != again {
		t.Fatal("same master and purpose must derive the same subkey")
	}
	if first == second {
		t.Fatal("different purposes must derive different subkeys")
	}
	if bytes.Equal(first[:], master) {
		t.Fatal("derived subkey must not equal the master key")
	}
}

func TestDeriveRejectsInvalidInputs(t *testing.T) {
	if _, err := Derive([]byte("short"), "gooru/test"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("short master error = %v, want ErrInvalidKey", err)
	}
	if _, err := Derive(bytes.Repeat([]byte{0x11}, Size), "  "); err == nil {
		t.Fatal("empty purpose must fail")
	}
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

func TestLoadAllowsNonWritableGroupOrOtherPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not authoritative on Windows")
	}

	for _, mode := range []os.FileMode{0440, 0640, 0444, 0644, 0511} {
		mode := mode
		t.Run(mode.String(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "key")
			if err := os.WriteFile(path, []byte(encodedKey(0x31)), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(Source{File: path}); err != nil {
				t.Fatalf("mode %04o should be accepted: %v", mode.Perm(), err)
			}
		})
	}
}

func TestLoadRejectsGroupOrOtherWritableKeyFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not authoritative on Windows")
	}

	for _, mode := range []os.FileMode{0620, 0602, 0622} {
		mode := mode
		t.Run(mode.String(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "key")
			if err := os.WriteFile(path, []byte(encodedKey(0x31)), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(Source{File: path}); err == nil {
				t.Fatalf("mode %04o should be rejected", mode.Perm())
			}
		})
	}
}
