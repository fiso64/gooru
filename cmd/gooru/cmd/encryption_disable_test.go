package cmd

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/gooru"
	"gooru.local/internal/database"
	"gooru.local/internal/encryptedfile"
	"gooru.local/internal/encryptionkeys"
	"gooru.local/internal/serve"
	"gooru.local/types"
)

func writeRecoveryKeyFile(t *testing.T, dir string, key []byte) string {
	t.Helper()
	path := filepath.Join(dir, "recovery.key")
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(key)), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func registerManagedPath(t *testing.T, dbPath, hash, path string, size int64) {
	t.Helper()
	store, err := database.NewStore(dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.DB.Exec(`INSERT INTO contents(hash) VALUES (?)`, hash); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec(`INSERT INTO locations(public_id, content_hash, path, size_bytes, mod_time, extension) VALUES (?, ?, ?, ?, ?, ?)`, "test-"+hash, hash, path, size, int64(1_700_000_000), filepath.Ext(path)); err != nil {
		t.Fatal(err)
	}
}

func TestConfiguredClientDisablesProtectedStorageRestartSafely(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	if err := gooru.Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatal(err)
	}
	uploadRoot := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(uploadRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(uploadRoot, "first.jpg")
	secondPath := filepath.Join(uploadRoot, "second.jpg")
	firstPlain := []byte("first managed plaintext")
	secondPlain := []byte("second managed plaintext")
	if err := os.WriteFile(firstPath, firstPlain, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, secondPlain, 0o644); err != nil {
		t.Fatal(err)
	}
	registerManagedPath(t, dbPath, "hash-first", firstPath, int64(len(firstPlain)))
	registerManagedPath(t, dbPath, "hash-second", secondPath, int64(len(secondPlain)))

	master := bytes.Repeat([]byte{0x7a}, 32)
	keys, err := encryptionkeys.Derive(master)
	if err != nil {
		t.Fatal(err)
	}
	if err := encryptedfile.EncryptFileInPlace(firstPath, keys.Media); err != nil {
		t.Fatal(err)
	}
	if err := encryptedfile.EncryptFileInPlace(secondPath, keys.Media); err != nil {
		t.Fatal(err)
	}
	if err := database.MigratePlaintextDatabase(dbPath, keys.Database); err != nil {
		t.Fatal(err)
	}
	protectedClient, err := gooru.NewWithOptions(dbPath, false, gooru.OpenOptions{Database: gooru.DatabaseOpenOptions{EncryptionKey: keys.Database}})
	if err != nil {
		t.Fatal(err)
	}
	files, err := protectedClient.GetAllFilesInfo()
	if err != nil {
		t.Fatal(err)
	}
	physicalByLogical := make(map[string]string, len(files))
	for _, file := range files {
		physical, err := serve.OpaqueManagedStoragePath(file.Path, file.Hash, keys.Media)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(file.Path, physical); err != nil {
			t.Fatal(err)
		}
		if err := protectedClient.SetManagedStoragePath(file.ID, physical); err != nil {
			t.Fatal(err)
		}
		physicalByLogical[file.Path] = physical
	}
	if err := protectedClient.Close(); err != nil {
		t.Fatal(err)
	}

	// Simulate a process dying after restoring one managed file and its canonical
	// filename but before clearing the mapping/database-last transition. The next
	// startup must recover that half-completed rename and finish the other file.
	firstPhysical := physicalByLogical[firstPath]
	if err := encryptedfile.DecryptFileInPlace(firstPhysical, keys.Media); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(firstPhysical, firstPath); err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(dir, "cache")
	protectedCache := filepath.Join(cacheRoot, "protected-v1", "aa", "cipher.jpg")
	if err := os.MkdirAll(filepath.Dir(protectedCache), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(protectedCache, []byte("disposable encrypted derivative"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := serve.DefaultConfig(dbPath)
	cfg.Encryption.Enabled = false
	cfg.Encryption.KeyFile = writeRecoveryKeyFile(t, dir, master)
	cfg.Uploads.Targets = []serve.UploadTarget{{ID: "managed", Name: "Managed", Path: uploadRoot}}
	cfg.Media.CacheDir = cacheRoot

	client, err := openConfiguredClient(cfg, false)
	if err != nil {
		t.Fatalf("disable protected storage: %v", err)
	}
	files, err := client.GetAllFilesInfo()
	if err != nil {
		_ = client.Close()
		t.Fatal(err)
	}
	if len(files) != 2 {
		_ = client.Close()
		t.Fatalf("registered files after transition = %d, want 2", len(files))
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}

	if plain, err := database.IsPlaintextDatabase(dbPath); err != nil || !plain {
		t.Fatalf("database plaintext after disable = %t, %v", plain, err)
	}
	for path, want := range map[string][]byte{firstPath: firstPlain, secondPath: secondPlain} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("restored managed file %q differs from plaintext", path)
		}
	}
	if _, err := os.Stat(filepath.Join(cacheRoot, "protected-v1")); !os.IsNotExist(err) {
		t.Fatalf("protected derivative namespace remains after disable: %v", err)
	}

	// Once the DB is plaintext the recovery key is no longer required for
	// ordinary operation; a subsequent startup without any key source succeeds.
	cfg.Encryption.KeyFile = ""
	client, err = openConfiguredClient(cfg, false)
	if err != nil {
		t.Fatalf("ordinary restart after completed disable: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestConfiguredDisableWrongRecoveryKeyFailsBeforeManagedMediaMutation(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	if err := gooru.Init(dbPath, types.StrategyPartial, false); err != nil {
		t.Fatal(err)
	}
	uploadRoot := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(uploadRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(uploadRoot, "managed.jpg")
	plaintext := []byte("must remain encrypted on wrong recovery key")
	if err := os.WriteFile(mediaPath, plaintext, 0o644); err != nil {
		t.Fatal(err)
	}
	registerManagedPath(t, dbPath, "hash-managed", mediaPath, int64(len(plaintext)))

	master := bytes.Repeat([]byte{0x4c}, 32)
	keys, err := encryptionkeys.Derive(master)
	if err != nil {
		t.Fatal(err)
	}
	if err := encryptedfile.EncryptFileInPlace(mediaPath, keys.Media); err != nil {
		t.Fatal(err)
	}
	if err := database.MigratePlaintextDatabase(dbPath, keys.Database); err != nil {
		t.Fatal(err)
	}
	beforeMedia, err := os.ReadFile(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeDB, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	cfg := serve.DefaultConfig(dbPath)
	cfg.Encryption.KeyFile = writeRecoveryKeyFile(t, dir, bytes.Repeat([]byte{0x4d}, 32))
	cfg.Uploads.Targets = []serve.UploadTarget{{ID: "managed", Name: "Managed", Path: uploadRoot}}
	if _, err := openConfiguredClient(cfg, false); err == nil {
		t.Fatal("wrong recovery key unexpectedly disabled protected storage")
	}
	afterMedia, err := os.ReadFile(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	afterDB, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterMedia, beforeMedia) {
		t.Fatal("wrong recovery key mutated managed ciphertext")
	}
	if !bytes.Equal(afterDB, beforeDB) {
		t.Fatal("wrong recovery key mutated encrypted database")
	}
}
