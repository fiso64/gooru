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
	params := map[string]uint64{}
	for _, part := range strings.Split(parts[3], ",") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return false, errors.New("invalid password hash parameters")
		}
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return false, errors.New("invalid password hash parameters")
		}
		params[k] = n
	}
	memory, mok := params["m"]
	iterations, tok := params["t"]
	parallelism, pok := params["p"]
	if !mok || !tok || !pok {
		return false, errors.New("missing password hash parameters")
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[4])
	if err != nil {
		return false, errors.New("invalid password salt")
	}
	want, err := enc.DecodeString(parts[5])
	if err != nil {
		return false, errors.New("invalid password hash")
	}
	got := argon2.IDKey([]byte(password), salt, uint32(iterations), uint32(memory), uint8(parallelism), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
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
