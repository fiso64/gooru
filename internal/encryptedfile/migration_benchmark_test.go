package encryptedfile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkEncryptFileInPlace16MiB(b *testing.B) {
	const size = 16 * ChunkSize
	plaintext := bytes.Repeat([]byte("gooru-migration-benchmark-"), size/len("gooru-migration-benchmark-")+1)
	plaintext = plaintext[:size]
	key := testKey(0x93)
	path := filepath.Join(b.TempDir(), "media.bin")

	b.SetBytes(size)
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		if err := os.WriteFile(path, plaintext, 0o600); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		if err := EncryptFileInPlace(path, key); err != nil {
			b.Fatal(err)
		}
	}
}
