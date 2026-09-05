package encryptionkeys

import (
	"bytes"
	"testing"
)

func TestDeriveSeparatesStableDomains(t *testing.T) {
	master := bytes.Repeat([]byte{0x42}, KeySize)
	first, err := Derive(master)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Derive(master)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Database, second.Database) || !bytes.Equal(first.Media, second.Media) {
		t.Fatal("subkey derivation is not stable")
	}
	if bytes.Equal(first.Database, master) || bytes.Equal(first.Media, master) {
		t.Fatal("derived subkey unexpectedly equals the master key")
	}
	if bytes.Equal(first.Database, first.Media) {
		t.Fatal("database and media domains produced the same subkey")
	}
}

func TestDeriveRejectsInvalidMasterLength(t *testing.T) {
	if _, err := Derive(make([]byte, KeySize-1)); err == nil {
		t.Fatal("expected invalid master key length to fail")
	}
}
