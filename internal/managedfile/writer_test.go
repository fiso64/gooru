package managedfile

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gooru.local/internal/encryptedfile"
)

func TestWriterFilesystemPersistsPlaintext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain.bin")
	file, err := os.Create(path)
	require.NoError(t, err)

	payload := []byte("managed plaintext")
	size, err := NewFilesystem().Write(file, bytes.NewReader(payload), 0)
	require.NoError(t, err)
	require.Equal(t, int64(len(payload)), size)
	require.NoError(t, file.Close())

	stored, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, payload, stored)
}

func TestWriterProtectedPersistsOnlyEncryptedContainer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "protected.bin")
	file, err := os.Create(path)
	require.NoError(t, err)

	key := bytes.Repeat([]byte{0x42}, 32)
	payload := []byte("logical managed content that must not persist as plaintext")
	size, err := NewProtected(key).Write(file, bytes.NewReader(payload), 0)
	require.NoError(t, err)
	require.Equal(t, int64(len(payload)), size)
	require.NoError(t, file.Close())

	stored, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(stored), string(payload))

	source, err := encryptedfile.Open(path, key)
	require.NoError(t, err)
	defer source.Close()
	decoded, err := io.ReadAll(source)
	require.NoError(t, err)
	require.Equal(t, payload, decoded)
}

func TestWriterProtectedInvalidKeyFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid-key.bin")
	file, err := os.Create(path)
	require.NoError(t, err)

	payload := []byte("must never be written as plaintext")
	_, err = NewProtected(nil).Write(file, bytes.NewReader(payload), 0)
	require.Error(t, err)
	require.NoError(t, file.Close())

	stored, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(stored), string(payload))
}

func TestWriterLimitUsesLogicalPlaintextSize(t *testing.T) {
	for _, protected := range []bool{false, true} {
		t.Run(map[bool]string{false: "filesystem", true: "protected"}[protected], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "limited.bin")
			file, err := os.Create(path)
			require.NoError(t, err)

			writer := NewFilesystem()
			if protected {
				writer = NewProtected(bytes.Repeat([]byte{0x24}, 32))
			}
			size, err := writer.Write(file, bytes.NewReader([]byte("123456")), 5)
			require.ErrorIs(t, err, ErrTooLarge)
			require.Equal(t, int64(6), size)
			require.NoError(t, file.Close())
		})
	}
}

func TestWriterPropagatesInputFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failed.bin")
	file, err := os.Create(path)
	require.NoError(t, err)
	defer file.Close()

	expected := errors.New("read failed")
	_, err = NewFilesystem().Write(file, errorReader{err: expected}, 0)
	require.ErrorIs(t, err, expected)
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }
