package serve

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryDerivativeStoreNeverPersistsPlaintext(t *testing.T) {
	store := memoryDerivativeStore{}
	artifact, err := store.GetOrGenerate(filepath.Join("aa", "cover.jpg"), func(dst io.Writer) error {
		_, err := io.WriteString(dst, "sensitive derivative")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	if artifact.CacheStatus != "bypass" || artifact.CacheControl != "private, no-store" {
		t.Fatalf("unexpected memory policy: status=%q control=%q", artifact.CacheStatus, artifact.CacheControl)
	}
	data, err := io.ReadAll(artifact.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "sensitive derivative" {
		t.Fatalf("unexpected artifact contents: %q", data)
	}
}

func TestPersistentDerivativeStoreGeneratesOnceAndReusesArtifact(t *testing.T) {
	root := t.TempDir()
	store := newPersistentDerivativeStore(root)
	generated := 0
	generate := func(dst io.Writer) error {
		generated++
		_, err := io.WriteString(dst, "cached derivative")
		return err
	}

	first, err := store.GetOrGenerate(filepath.Join("ab", "cover.jpg"), generate)
	if err != nil {
		t.Fatal(err)
	}
	if first.CacheStatus != "miss" {
		t.Fatalf("first status = %q, want miss", first.CacheStatus)
	}
	_ = first.Close()

	second, err := store.GetOrGenerate(filepath.Join("ab", "cover.jpg"), generate)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.CacheStatus != "hit" {
		t.Fatalf("second status = %q, want hit", second.CacheStatus)
	}
	if generated != 1 {
		t.Fatalf("generator called %d times, want 1", generated)
	}
	data, err := io.ReadAll(second.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "cached derivative" {
		t.Fatalf("unexpected cached artifact: %q", data)
	}
	info, err := os.Stat(filepath.Join(root, "ab", "cover.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("artifact permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestEncryptedDerivativeStorePersistsCiphertextAndReusesArtifact(t *testing.T) {
	root := t.TempDir()
	key := bytes.Repeat([]byte{0x42}, 32)
	store := newEncryptedDerivativeStore(root, key)
	generated := 0
	const plaintext = "sensitive derivative bytes that must never appear in the cache"
	generate := func(dst io.Writer) error {
		generated++
		_, err := io.WriteString(dst, plaintext)
		return err
	}

	first, err := store.GetOrGenerate(filepath.Join("ab", "cover.jpg"), generate)
	if err != nil {
		t.Fatal(err)
	}
	if first.CacheStatus != "miss" || first.CacheControl != "private, no-store" {
		t.Fatalf("unexpected first policy: status=%q control=%q", first.CacheStatus, first.CacheControl)
	}
	firstData, err := io.ReadAll(first.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Close()
	if string(firstData) != plaintext {
		t.Fatalf("unexpected decrypted derivative: %q", firstData)
	}

	cachePath := filepath.Join(root, encryptedDerivativeNamespace, "ab", "cover.jpg")
	ciphertext, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte(plaintext)) {
		t.Fatal("protected derivative cache contains plaintext derivative bytes")
	}
	info, err := os.Stat(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("artifact permissions = %o, want 600", info.Mode().Perm())
	}

	second, err := store.GetOrGenerate(filepath.Join("ab", "cover.jpg"), generate)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.CacheStatus != "hit" {
		t.Fatalf("second status = %q, want hit", second.CacheStatus)
	}
	if generated != 1 {
		t.Fatalf("generator called %d times, want 1", generated)
	}
	secondData, err := io.ReadAll(second.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(secondData) != plaintext {
		t.Fatalf("unexpected cached derivative: %q", secondData)
	}
}

func TestEncryptedDerivativeStoreRegeneratesUnreadableCacheEntry(t *testing.T) {
	root := t.TempDir()
	store := newEncryptedDerivativeStore(root, bytes.Repeat([]byte{0x51}, 32))
	cachePath := filepath.Join(root, encryptedDerivativeNamespace, "ab", "cover.jpg")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, []byte("not an encrypted derivative"), 0600); err != nil {
		t.Fatal(err)
	}

	generated := 0
	artifact, err := store.GetOrGenerate(filepath.Join("ab", "cover.jpg"), func(dst io.Writer) error {
		generated++
		_, err := io.WriteString(dst, "regenerated derivative")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	if artifact.CacheStatus != "miss" {
		t.Fatalf("status = %q, want miss", artifact.CacheStatus)
	}
	if generated != 1 {
		t.Fatalf("generator called %d times, want 1", generated)
	}
	data, err := io.ReadAll(artifact.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "regenerated derivative" {
		t.Fatalf("unexpected regenerated derivative: %q", data)
	}
	ciphertext, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("regenerated derivative")) {
		t.Fatal("regenerated protected cache entry contains plaintext")
	}
}

func TestNewDerivativeStoreChoosesPersistenceAtCompositionBoundary(t *testing.T) {
	plainCfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	plainCfg.Media.CacheDir = t.TempDir()
	plain, err := newDerivativeStore(plainCfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := plain.(*persistentDerivativeStore); !ok {
		t.Fatalf("plain store type = %T, want persistentDerivativeStore", plain)
	}

	protectedCfg := plainCfg
	protectedCfg.Encryption.Enabled = true
	protectedCfg.Encryption.Key = bytes.Repeat([]byte{0x33}, 32)
	protected, err := newDerivativeStore(protectedCfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := protected.(*encryptedDerivativeStore); !ok {
		t.Fatalf("protected store type = %T, want encryptedDerivativeStore", protected)
	}
}

func TestProtectedDerivativeStoreWritesOnlyEncryptedCacheEntries(t *testing.T) {
	cacheDir := t.TempDir()
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Media.CacheDir = cacheDir
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x24}, 32)
	store, err := newDerivativeStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	const plaintext = "sensitive derivative"
	artifact, err := store.GetOrGenerate(filepath.Join("ab", "cover.jpg"), func(dst io.Writer) error {
		_, err := io.WriteString(dst, plaintext)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = artifact.Close()
	data, err := os.ReadFile(filepath.Join(cacheDir, encryptedDerivativeNamespace, "ab", "cover.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(plaintext)) {
		t.Fatal("protected derivative cache persisted plaintext bytes")
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "ab", "cover.jpg")); !os.IsNotExist(err) {
		t.Fatalf("protected cache collided with plaintext derivative namespace: %v", err)
	}
}
