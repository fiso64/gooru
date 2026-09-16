package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
	"gooru.local/gooru"
	"gooru.local/internal/database"
	"gooru.local/types"
)

const (
	devUsername = "admin"
	devPassword = "123"
)

func main() {
	reset := flag.Bool("reset", false, "remove the existing dev database before seeding")
	flag.Parse()

	root, err := repoRoot()
	must(err)

	devDir := filepath.Join(root, ".dev")
	dbPath := filepath.Join(devDir, "gooru.db")
	uploadsDir := filepath.Join(devDir, "uploads")
	mediaCacheDir := filepath.Join(devDir, "media-cache")
	configPath := filepath.Join(devDir, "serve.yaml")

	must(os.MkdirAll(uploadsDir, 0755))
	must(os.MkdirAll(mediaCacheDir, 0755))

	if *reset {
		must(removeDBFiles(dbPath))
	}
	if _, err := os.Stat(dbPath); errors.Is(err, os.ErrNotExist) {
		must(gooru.Init(dbPath, types.StrategyPartial, false))
	} else {
		must(err)
	}

	store, err := database.NewStore(dbPath, false)
	must(err)
	defer func() { must(store.Close()) }()
	must(database.RunMigrations(store.DB))
	must(ensureHashingStrategy(store))
	must(upsertDevAdmin(context.Background(), store.DB))
	must(database.SecureDBFiles(dbPath))
	must(writeConfig(configPath, dbPath, uploadsDir, mediaCacheDir))

	fmt.Printf("dev profile ready\n")
	fmt.Printf("  config:  %s\n", configPath)
	fmt.Printf("  db:      %s\n", dbPath)
	fmt.Printf("  uploads: %s\n", uploadsDir)
	fmt.Printf("  login:   %s / %s\n", devUsername, devPassword)
	fmt.Printf("\nRun it with:\n  go run ./cmd/gooru serve --config %s\n", configPath)
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "", errors.New("could not find repo root containing go.mod")
		}
		wd = parent
	}
}

func removeDBFiles(dbPath string) error {
	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func ensureHashingStrategy(store *database.Store) error {
	if _, err := store.GetHashingStrategy(); err == nil {
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return store.SetHashingStrategy(types.StrategyPartial)
}

func upsertDevAdmin(ctx context.Context, db *sql.DB) error {
	hash, err := hashPasswordForDev(devPassword)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	id, err := publicID("usr")
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO users (id, username, password_hash, role, created_at, updated_at, disabled_at)
VALUES (?, ?, ?, 'admin', ?, ?, NULL)
ON CONFLICT(username) DO UPDATE SET
  password_hash = excluded.password_hash,
  role = 'admin',
  updated_at = excluded.updated_at,
  disabled_at = NULL`, id, devUsername, hash, now, now)
	return err
}

func hashPasswordForDev(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 1, 32)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=1$%s$%s", enc.EncodeToString(salt), enc.EncodeToString(hash)), nil
}

func publicID(prefix string) (string, error) {
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(raw), nil
}

func writeConfig(path, dbPath, uploadsDir, mediaCacheDir string) error {
	data := fmt.Sprintf(`server:
  listen: "127.0.0.1:5678"
  frontend_dir: "frontend/build"
  max_request_body_bytes: 104857600

database:
  path: %q

auth:
  enabled: true
  session_ttl: "720h"
  cookie_name: "gooru_session"
  cookie_secure: "auto"
  cookie_same_site: "lax"

uploads:
  enabled: true
  targets:
    - id: "dev"
      name: "Dev uploads"
      path: %q
  max_file_size_bytes: 104857600
  max_queued: 100
media:
  cache_dir: %q
  thumbnail_sizes: [256, 512]
  thumbnail_format: "jpeg"
  preview_size: 1280


tools:
  ffmpeg_path: "ffmpeg"
  ffprobe_path: "ffprobe"

logging:
  level: "info"
`, filepath.ToSlash(dbPath), filepath.ToSlash(uploadsDir), filepath.ToSlash(mediaCacheDir))
	return os.WriteFile(path, []byte(data), 0600)
}

func must(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, strings.TrimSpace(err.Error()))
	os.Exit(1)
}
