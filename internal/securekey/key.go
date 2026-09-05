package securekey

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
)

const (
	Size       = 32
	EnvKey     = "GOORU_ENCRYPTION_KEY"
	EnvKeyFile = "GOORU_ENCRYPTION_KEY_FILE"
)

var ErrInvalidKey = errors.New("encryption key must decode to exactly 32 bytes")

// Source identifies where an encryption key should be loaded from. Keeping the
// key itself out of YAML avoids committing secrets while allowing database and
// upload encryption to share one resolution path.
type Source struct {
	Env  string
	File string
}

// Derive returns a purpose-bound 256-bit subkey from the process encryption key.
// Feature code should prefer a derived subkey over using the master key directly
// so compromise or misuse of one ciphertext domain cannot silently cross domains.
func Derive(master []byte, purpose string) ([Size]byte, error) {
	var derived [Size]byte
	if len(master) != Size {
		return derived, ErrInvalidKey
	}
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return derived, errors.New("encryption subkey purpose must not be empty")
	}
	mac := hmac.New(sha256.New, master)
	_, _ = mac.Write([]byte(purpose))
	copy(derived[:], mac.Sum(nil))
	return derived, nil
}

// LoadProcess resolves Gooru's process-wide encryption key contract. Encryption
// is disabled when neither source is configured; setting either source opts in.
func LoadProcess() (key []byte, enabled bool, err error) {
	_, hasEnv := os.LookupEnv(EnvKey)
	file := strings.TrimSpace(os.Getenv(EnvKeyFile))
	if !hasEnv && file == "" {
		return nil, false, nil
	}
	source := Source{File: file}
	if hasEnv {
		source.Env = EnvKey
	}
	key, err = Load(source)
	if err != nil {
		return nil, true, err
	}
	return key, true, nil
}

// Load resolves one base64-encoded 256-bit key. Exactly one source must be set.
// File contents and environment values may contain surrounding whitespace.
func Load(source Source) ([]byte, error) {
	envName := strings.TrimSpace(source.Env)
	fileName := strings.TrimSpace(source.File)
	if (envName == "") == (fileName == "") {
		return nil, errors.New("configure exactly one encryption key source")
	}

	var encoded string
	if envName != "" {
		value, ok := os.LookupEnv(envName)
		if !ok {
			return nil, fmt.Errorf("encryption key environment variable %q is not set", envName)
		}
		encoded = value
	} else {
		file, err := os.Open(fileName)
		if err != nil {
			return nil, fmt.Errorf("open encryption key file: %w", err)
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return nil, fmt.Errorf("inspect encryption key file: %w", err)
		}
		if !info.Mode().IsRegular() {
			return nil, errors.New("encryption key file must be a regular file")
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			return nil, errors.New("encryption key file must not be readable or writable by group or others")
		}
		data, err := io.ReadAll(file)
		if err != nil {
			return nil, fmt.Errorf("read encryption key file: %w", err)
		}
		encoded = string(data)
	}

	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, fmt.Errorf("decode encryption key: %w", err)
	}
	if len(key) != Size {
		return nil, ErrInvalidKey
	}
	return key, nil
}
