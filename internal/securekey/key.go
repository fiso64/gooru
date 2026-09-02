package securekey

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

const Size = 32

var ErrInvalidKey = errors.New("encryption key must decode to exactly 32 bytes")

// Source identifies where an encryption key should be loaded from. Keeping the
// key itself out of YAML avoids committing secrets while allowing database and
// upload encryption to share one resolution path.
type Source struct {
	Env  string
	File string
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
		data, err := os.ReadFile(fileName)
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
