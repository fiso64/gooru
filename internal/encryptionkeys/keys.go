package encryptionkeys

import (
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

const KeySize = 32

const (
	databaseDomain = "gooru/encryption/database/v1"
	mediaDomain    = "gooru/encryption/media/v1"
)

type Keys struct {
	Database []byte
	Media    []byte
}

func Derive(master []byte) (Keys, error) {
	if len(master) != KeySize {
		return Keys{}, fmt.Errorf("master encryption key must be exactly %d bytes", KeySize)
	}
	database, err := derive(master, databaseDomain)
	if err != nil {
		return Keys{}, err
	}
	media, err := derive(master, mediaDomain)
	if err != nil {
		return Keys{}, err
	}
	return Keys{Database: database, Media: media}, nil
}

func derive(master []byte, domain string) ([]byte, error) {
	key := make([]byte, KeySize)
	reader := hkdf.New(sha256.New, master, nil, []byte(domain))
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("derive %s subkey: %w", domain, err)
	}
	return key, nil
}
