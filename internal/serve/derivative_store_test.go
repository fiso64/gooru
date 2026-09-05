package serve

import (
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
	protected, err := newDerivativeStore(protectedCfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := protected.(memoryDerivativeStore); !ok {
		t.Fatalf("protected store type = %T, want memoryDerivativeStore", protected)
	}
}

func TestProtectedDerivativeStoreLeavesConfiguredCacheUntouched(t *testing.T) {
	cacheDir := t.TempDir()
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Media.CacheDir = cacheDir
	cfg.Encryption.Enabled = true
	store, err := newDerivativeStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := store.GetOrGenerate(filepath.Join("ab", "cover.jpg"), func(dst io.Writer) error {
		_, err := io.WriteString(dst, "sensitive derivative")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = artifact.Close()
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("protected derivative store persisted plaintext cache entries: %v", entries)
	}
}
