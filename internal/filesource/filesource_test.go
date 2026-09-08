package filesource

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/internal/encryptedfile"
)

func TestProtectedResolverReturnsLogicalPlaintext(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "media.bin")
	plaintext := []byte("logical plaintext media")
	key := bytes.Repeat([]byte{0x42}, 32)
	writeEncryptedTestFile(t, path, plaintext, key)

	resolver, err := NewProtected(key, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	source, err := resolver.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if source.Size() != int64(len(plaintext)) {
		t.Fatalf("logical size = %d, want %d", source.Size(), len(plaintext))
	}
	got, err := io.ReadAll(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("logical contents = %q, want %q", got, plaintext)
	}
}

func TestProtectedResolverKeepsExternalFilesPlaintext(t *testing.T) {
	root := t.TempDir()
	externalRoot := t.TempDir()
	path := filepath.Join(externalRoot, "external.bin")
	plaintext := []byte("external plaintext")
	if err := os.WriteFile(path, plaintext, 0o600); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewProtected(bytes.Repeat([]byte{0x23}, 32), []string{root})
	if err != nil {
		t.Fatal(err)
	}
	source, err := resolver.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	got, err := io.ReadAll(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("contents = %q, want %q", got, plaintext)
	}
}

func TestProtectedResolverAuthenticatesEncryptedFileOutsideCurrentRoots(t *testing.T) {
	root := t.TempDir()
	externalRoot := t.TempDir()
	path := filepath.Join(externalRoot, "previously-managed.bin")
	plaintext := []byte("encrypted tracked content outside current root")
	key := bytes.Repeat([]byte{0x66}, 32)
	writeEncryptedTestFile(t, path, plaintext, key)

	resolver, err := NewProtected(key, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	source, err := resolver.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	got, err := io.ReadAll(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("contents = %q, want %q", got, plaintext)
	}
}

func TestProtectedResolverRejectsPlaintextManagedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "plaintext.bin")
	if err := os.WriteFile(path, []byte("must not leak"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewProtected(bytes.Repeat([]byte{0x31}, 32), []string{root})
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolver.Open(path)
	if !errors.Is(err, encryptedfile.ErrInvalidFormat) {
		t.Fatalf("Open() error = %v, want invalid encrypted format", err)
	}
}

func TestProtectedPathDoesNotMatchSiblingPrefix(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "managed")
	sibling := filepath.Join(base, "managed-other", "file.bin")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewProtected(bytes.Repeat([]byte{0x51}, 32), []string{root})
	if err != nil {
		t.Fatal(err)
	}
	if resolver.IsProtectedPath(sibling) {
		t.Fatalf("sibling-prefix path %q was treated as protected", sibling)
	}
}

func writeEncryptedTestFile(t testing.TB, path string, plaintext, key []byte) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := encryptedfile.Encrypt(file, bytes.NewReader(plaintext), int64(len(plaintext)), key); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func protectedBenchmarkFixture(b *testing.B, plaintext []byte) (*Resolver, string) {
	b.Helper()
	root := b.TempDir()
	path := filepath.Join(root, "media.bin")
	key := bytes.Repeat([]byte{0xa7}, 32)
	writeEncryptedTestFile(b, path, plaintext, key)
	resolver, err := NewProtected(key, []string{root})
	if err != nil {
		b.Fatal(err)
	}
	return resolver, path
}

func benchmarkPlaintext(size int) []byte {
	out := make([]byte, size)
	for i := range out {
		out[i] = byte((i*73 + i/251 + 19) % 251)
	}
	return out
}

func BenchmarkProtectedSourceSequential32KiB(b *testing.B) {
	plaintext := benchmarkPlaintext(16 * encryptedfile.ChunkSize)
	resolver, path := protectedBenchmarkFixture(b, plaintext)
	buffer := make([]byte, 32<<10)
	b.SetBytes(int64(len(plaintext)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		source, err := resolver.Open(path)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := io.CopyBuffer(io.Discard, source, buffer); err != nil {
			_ = source.Close()
			b.Fatal(err)
		}
		if err := source.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

type discardResponseWriter struct {
	header http.Header
	status int
}

func (w *discardResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *discardResponseWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *discardResponseWriter) WriteHeader(status int)      { w.status = status }

func BenchmarkProtectedSourceHTTPRange4MiB(b *testing.B) {
	plaintext := benchmarkPlaintext(32 * encryptedfile.ChunkSize)
	resolver, path := protectedBenchmarkFixture(b, plaintext)
	req, err := http.NewRequest(http.MethodGet, "http://gooru.test/media", nil)
	if err != nil {
		b.Fatal(err)
	}
	req.Header.Set("Range", "bytes=4194304-8388607")
	b.SetBytes(4 * encryptedfile.ChunkSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		source, err := resolver.Open(path)
		if err != nil {
			b.Fatal(err)
		}
		writer := &discardResponseWriter{}
		http.ServeContent(writer, req, "media.mp4", source.ModTime(), source)
		if err := source.Close(); err != nil {
			b.Fatal(err)
		}
		if writer.status != http.StatusPartialContent {
			b.Fatalf("ServeContent status = %d, want %d", writer.status, http.StatusPartialContent)
		}
	}
}

func BenchmarkProtectedSourceCBZPages(b *testing.B) {
	archive := benchmarkCBZ(b)
	resolver, path := protectedBenchmarkFixture(b, archive)
	const pagesPerIteration = 3
	b.SetBytes(pagesPerIteration * 256 << 10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		source, err := resolver.Open(path)
		if err != nil {
			b.Fatal(err)
		}
		reader, err := zip.NewReader(source, source.Size())
		if err != nil {
			_ = source.Close()
			b.Fatal(err)
		}
		for _, page := range []int{0, len(reader.File) / 2, len(reader.File) - 1} {
			opened, err := reader.File[page].Open()
			if err != nil {
				_ = source.Close()
				b.Fatal(err)
			}
			if _, err := io.Copy(io.Discard, opened); err != nil {
				_ = opened.Close()
				_ = source.Close()
				b.Fatal(err)
			}
			if err := opened.Close(); err != nil {
				_ = source.Close()
				b.Fatal(err)
			}
		}
		if err := source.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProtectedSourceParallel4MiB(b *testing.B) {
	plaintext := benchmarkPlaintext(16 * encryptedfile.ChunkSize)
	resolver, path := protectedBenchmarkFixture(b, plaintext)
	b.SetBytes(4 * encryptedfile.ChunkSize)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		buffer := make([]byte, 32<<10)
		for pb.Next() {
			source, err := resolver.Open(path)
			if err != nil {
				b.Error(err)
				return
			}
			if _, err := io.CopyBuffer(io.Discard, io.LimitReader(source, 4*encryptedfile.ChunkSize), buffer); err != nil {
				_ = source.Close()
				b.Error(err)
				return
			}
			if err := source.Close(); err != nil {
				b.Error(err)
				return
			}
		}
	})
}

func benchmarkCBZ(b *testing.B) []byte {
	b.Helper()
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	page := benchmarkPlaintext(256 << 10)
	for i := 0; i < 32; i++ {
		header := &zip.FileHeader{Name: filepath.ToSlash(filepath.Join("pages", pageName(i))), Method: zip.Store}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := entry.Write(page); err != nil {
			b.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		b.Fatal(err)
	}
	return out.Bytes()
}

func pageName(index int) string {
	const digits = "0123456789"
	return "page-" + string([]byte{
		digits[(index/10)%10],
		digits[index%10],
	}) + ".jpg"
}
