package serve

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	passwordHashMemory      uint32 = 64 * 1024
	passwordHashIterations  uint32 = 3
	passwordHashParallelism uint8  = 1
	passwordHashSaltBytes          = 16
	passwordHashKeyBytes           = 32
)

func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, passwordHashSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, passwordHashIterations, passwordHashMemory, passwordHashParallelism, passwordHashKeyBytes)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		passwordHashMemory,
		passwordHashIterations,
		passwordHashParallelism,
		enc.EncodeToString(salt),
		enc.EncodeToString(hash),
	), nil
}

func VerifyPassword(encoded string, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, errors.New("unsupported password hash format")
	}

	params, err := parseSupportedPasswordHashParams(parts[3])
	if err != nil {
		return false, err
	}
	if params["m"] != uint64(passwordHashMemory) || params["t"] != uint64(passwordHashIterations) || params["p"] != uint64(passwordHashParallelism) {
		return false, errors.New("unsupported password hash parameters")
	}

	enc := base64.RawStdEncoding
	if len(parts[4]) != enc.EncodedLen(passwordHashSaltBytes) || len(parts[5]) != enc.EncodedLen(passwordHashKeyBytes) {
		return false, errors.New("unsupported password hash size")
	}
	salt, err := enc.DecodeString(parts[4])
	if err != nil || len(salt) != passwordHashSaltBytes {
		return false, errors.New("invalid password salt")
	}
	want, err := enc.DecodeString(parts[5])
	if err != nil || len(want) != passwordHashKeyBytes {
		return false, errors.New("invalid password hash")
	}

	got := argon2.IDKey([]byte(password), salt, passwordHashIterations, passwordHashMemory, passwordHashParallelism, passwordHashKeyBytes)
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func parseSupportedPasswordHashParams(encoded string) (map[string]uint64, error) {
	parts := strings.Split(encoded, ",")
	if len(parts) != 3 {
		return nil, errors.New("invalid password hash parameters")
	}
	params := make(map[string]uint64, 3)
	for _, part := range parts {
		key, value, ok := strings.Cut(part, "=")
		if !ok || (key != "m" && key != "t" && key != "p") {
			return nil, errors.New("invalid password hash parameters")
		}
		if _, exists := params[key]; exists {
			return nil, errors.New("invalid password hash parameters")
		}
		n, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return nil, errors.New("invalid password hash parameters")
		}
		params[key] = n
	}
	if _, ok := params["m"]; !ok {
		return nil, errors.New("missing password hash parameters")
	}
	if _, ok := params["t"]; !ok {
		return nil, errors.New("missing password hash parameters")
	}
	if _, ok := params["p"]; !ok {
		return nil, errors.New("missing password hash parameters")
	}
	return params, nil
}

// ValidatePassword enforces only that an account actually has a password.
// Password composition and length are deliberately left to the user; Argon2id
// provides the storage-side protection independently of that policy choice.
func ValidatePassword(password string) error {
	if password == "" {
		return errors.New("password is required")
	}
	return nil
}
