package encryptedfile

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

var ErrAlreadyEncrypted = errors.New("file is already encrypted")

// EncryptFileInPlace converts one plaintext regular file to the encrypted-file
// container without ever writing plaintext to a sibling staging file. The
// encrypted replacement is fully written, synced, reopened, and authenticated
// before it can replace the source. The original modification time is retained
// so library file-status checks do not interpret the migration itself as media
// content changing.
//
// If the file already carries Gooru's encrypted-file magic, it is authenticated
// with key and ErrAlreadyEncrypted is returned. A wrong key or corrupt encrypted
// file therefore fails closed instead of being encrypted a second time.
func EncryptFileInPlace(path string, key []byte) error {
	source, err := os.Open(path)
	if err != nil {
		return err
	}
	sourceClosed := false
	defer func() {
		if !sourceClosed {
			_ = source.Close()
		}
	}()

	info, err := source.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: path is not a regular file", ErrInvalidFormat)
	}

	prefix := make([]byte, len(magic))
	n, readErr := io.ReadFull(source, prefix)
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return fmt.Errorf("inspect source prefix: %w", readErr)
	}
	if n == len(prefix) && bytes.Equal(prefix, []byte(magic)) {
		if err := source.Close(); err != nil {
			return err
		}
		sourceClosed = true
		opened, err := Open(path, key)
		if err != nil {
			return fmt.Errorf("authenticate existing encrypted file: %w", err)
		}
		if err := opened.Close(); err != nil {
			return err
		}
		return ErrAlreadyEncrypted
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, "."+base+".encrypted-")
	if err != nil {
		return fmt.Errorf("create encrypted sibling: %w", err)
	}
	tmpPath := tmp.Name()
	cleanupTemp := true
	defer func() {
		_ = tmp.Close()
		if cleanupTemp {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("secure encrypted sibling: %w", err)
	}
	if err := Encrypt(tmp, source, info.Size(), key); err != nil {
		return fmt.Errorf("encrypt source: %w", err)
	}
	if err := source.Close(); err != nil {
		return fmt.Errorf("close plaintext source before replacement: %w", err)
	}
	sourceClosed = true
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync encrypted sibling: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close encrypted sibling: %w", err)
	}

	verified, err := Open(tmpPath, key)
	if err != nil {
		return fmt.Errorf("reopen encrypted sibling: %w", err)
	}
	if _, err := io.Copy(io.Discard, verified); err != nil {
		_ = verified.Close()
		return fmt.Errorf("verify encrypted sibling: %w", err)
	}
	if err := verified.Close(); err != nil {
		return fmt.Errorf("close verified encrypted sibling: %w", err)
	}
	if err := os.Chtimes(tmpPath, time.Now(), info.ModTime()); err != nil {
		return fmt.Errorf("preserve source modification time: %w", err)
	}

	if err := replaceFile(tmpPath, path); err != nil {
		return err
	}
	cleanupTemp = false
	return nil
}

// ReencryptFileInPlace atomically rewrites an existing encrypted file from
// oldKey to newKey without materializing plaintext on disk. It is used for
// internal encryption-format/key-domain migrations rather than user-driven key
// rotation. The original file remains untouched until the replacement has been
// fully written, synced, reopened, and authenticated with newKey.
func ReencryptFileInPlace(path string, oldKey, newKey []byte) error {
	if bytes.Equal(oldKey, newKey) {
		opened, err := Open(path, newKey)
		if err != nil {
			return err
		}
		return opened.Close()
	}

	source, err := Open(path, oldKey)
	if err != nil {
		return fmt.Errorf("open encrypted source with legacy key: %w", err)
	}
	sourceClosed := false
	defer func() {
		if !sourceClosed {
			_ = source.Close()
		}
	}()

	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, "."+base+".rekeyed-")
	if err != nil {
		return fmt.Errorf("create rekeyed sibling: %w", err)
	}
	tmpPath := tmp.Name()
	cleanupTemp := true
	defer func() {
		_ = tmp.Close()
		if cleanupTemp {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("secure rekeyed sibling: %w", err)
	}
	if err := Encrypt(tmp, source, source.Size(), newKey); err != nil {
		return fmt.Errorf("reencrypt source: %w", err)
	}
	if err := source.Close(); err != nil {
		return fmt.Errorf("close legacy-key source before replacement: %w", err)
	}
	sourceClosed = true
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync rekeyed sibling: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close rekeyed sibling: %w", err)
	}

	verified, err := Open(tmpPath, newKey)
	if err != nil {
		return fmt.Errorf("reopen rekeyed sibling: %w", err)
	}
	if _, err := io.Copy(io.Discard, verified); err != nil {
		_ = verified.Close()
		return fmt.Errorf("verify rekeyed sibling: %w", err)
	}
	if err := verified.Close(); err != nil {
		return fmt.Errorf("close verified rekeyed sibling: %w", err)
	}
	if err := os.Chtimes(tmpPath, time.Now(), source.ModTime()); err != nil {
		return fmt.Errorf("preserve source modification time: %w", err)
	}
	if err := replaceFile(tmpPath, path); err != nil {
		return err
	}
	cleanupTemp = false
	return nil
}

func replaceFile(replacement, target string) error {
	if err := os.Rename(replacement, target); err == nil {
		return syncDir(filepath.Dir(target))
	}

	dir := filepath.Dir(target)
	backup, err := os.CreateTemp(dir, "."+filepath.Base(target)+".plaintext-backup-")
	if err != nil {
		return fmt.Errorf("reserve plaintext backup: %w", err)
	}
	backupPath := backup.Name()
	if err := backup.Close(); err != nil {
		_ = os.Remove(backupPath)
		return fmt.Errorf("close plaintext backup reservation: %w", err)
	}
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("clear plaintext backup reservation: %w", err)
	}
	if err := os.Rename(target, backupPath); err != nil {
		return fmt.Errorf("stage plaintext source for replacement: %w", err)
	}
	if err := os.Rename(replacement, target); err != nil {
		_ = os.Rename(backupPath, target)
		return fmt.Errorf("install encrypted replacement: %w", err)
	}
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("remove plaintext migration backup: %w", err)
	}
	return syncDir(dir)
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}
