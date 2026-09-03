from pathlib import Path


def replace(path, old, new):
    p = Path(path)
    text = p.read_text()
    if old not in text:
        raise SystemExit(f"expected text not found in {path}: {old!r}")
    p.write_text(text.replace(old, new, 1))


replace(
    "internal/serve/browse.go",
    "type GooruLibrary struct {\n\tclient   *core.Client\n\tverbose  bool\n\tmetadata MediaMetadataProvider\n}",
    "type GooruLibrary struct {\n\tclient     *core.Client\n\tverbose    bool\n\tmetadata   MediaMetadataProvider\n\tencryption EncryptionConfig\n}",
)
replace(
    "internal/serve/server.go",
    "if gooruLibrary, ok := library.(*GooruLibrary); ok {\n\t\tgooruLibrary.metadata = metadata\n\t}",
    "if gooruLibrary, ok := library.(*GooruLibrary); ok {\n\t\tgooruLibrary.metadata = metadata\n\t\tgooruLibrary.encryption = cfg.Encryption\n\t}",
)
replace(
    "internal/serve/uploads.go",
    "size, copyErr := copyUpload(dst, src, s.cfg.Uploads.MaxFileSizeBytes)",
    "size, copyErr := s.persistUploadedFile(dst, src)",
)
replace(
    "internal/serve/uploads.go",
    "analysisSource, analysisSize, analysisModTime, err := openUploadAnalysisSource(analysisPath)",
    "analysisSource, analysisSize, analysisModTime, err := l.openUploadAnalysisSource(analysisPath)",
)
replace(
    "internal/serve/uploads.go",
    "metadata, err := importedMediaMetadata(ctx, provider, file, analysisPaths[file.Path], mediaType, mediaKind)",
    "metadata, err := l.importedMediaMetadata(ctx, provider, file, analysisPaths[file.Path], mediaType, mediaKind)",
)

Path("internal/serve/upload_analysis_source.go").write_text('''package serve

import (
\t"context"
\t"fmt"
\t"io"
\t"os"

\t"gooru.local/internal/encryptedfile"
\t"gooru.local/types"
)

type uploadAnalysisSource interface {
\tio.ReaderAt
\tio.Closer
}

func (l *GooruLibrary) openUploadAnalysisSource(path string) (uploadAnalysisSource, int64, int64, error) {
\tif l.encryption.Enabled {
\t\tfile, err := encryptedfile.Open(path, l.encryption.Key)
\t\tif err != nil {
\t\t\treturn nil, 0, 0, err
\t\t}
\t\treturn file, file.Size(), file.ModTime().Unix(), nil
\t}
\tfile, err := os.Open(path)
\tif err != nil {
\t\treturn nil, 0, 0, err
\t}
\tinfo, err := file.Stat()
\tif err != nil {
\t\t_ = file.Close()
\t\treturn nil, 0, 0, err
\t}
\tif info.IsDir() {
\t\t_ = file.Close()
\t\treturn nil, 0, 0, fmt.Errorf("upload analysis source is a directory")
\t}
\treturn file, info.Size(), info.ModTime().Unix(), nil
}

func (l *GooruLibrary) importedMediaMetadata(ctx context.Context, provider MediaMetadataProvider, file types.FileInfo, analysisPath string, mediaType string, mediaKind string) (MediaMetadata, error) {
\tif sourceProvider, ok := provider.(MediaMetadataSourceProvider); ok && analysisPath != "" {
\t\tsource, size, _, err := l.openUploadAnalysisSource(analysisPath)
\t\tif err != nil {
\t\t\treturn MediaMetadata{}, err
\t\t}
\t\tdefer source.Close()
\t\treturn sourceProvider.MetadataFromSource(ctx, file, source, size, mediaType, mediaKind)
\t}
\tanalysisFile := file
\tif analysisPath != "" {
\t\tanalysisFile.Path = analysisPath
\t}
\treturn provider.Metadata(ctx, analysisFile, mediaType, mediaKind)
}
''')

Path("internal/serve/upload_encryption.go").write_text('''package serve

import (
\t"io"
\t"os"

\t"gooru.local/internal/encryptedfile"
)

func (s *Server) persistUploadedFile(dst *os.File, src io.Reader) (int64, error) {
\tif !s.cfg.Encryption.Enabled {
\t\treturn copyUpload(dst, src, s.cfg.Uploads.MaxFileSizeBytes)
\t}
\tif s.cfg.Uploads.MaxFileSizeBytes <= 0 {
\t\treturn encryptedfile.EncryptStream(dst, src, s.cfg.Encryption.Key)
\t}
\tlimited := &io.LimitedReader{R: src, N: s.cfg.Uploads.MaxFileSizeBytes + 1}
\tsize, err := encryptedfile.EncryptStream(dst, limited, s.cfg.Encryption.Key)
\tif err != nil {
\t\treturn size, err
\t}
\tif size > s.cfg.Uploads.MaxFileSizeBytes {
\t\treturn size, errUploadTooLarge
\t}
\treturn size, nil
}
''')

Path("internal/serve/upload_encryption_e2e_test.go").write_text('''package serve

import (
\t"bytes"
\t"image/color"
\t"io"
\t"net/http"
\t"net/http/httptest"
\t"os"
\t"path/filepath"
\t"testing"

\tcore "gooru.local/gooru"
\t"gooru.local/internal/encryptedfile"
\t"gooru.local/internal/securekey"
\t"gooru.local/types"
)

func TestEncryptedUploadImportAndContentGoldenPath(t *testing.T) {
\tdir := t.TempDir()
\tdbPath := filepath.Join(dir, "gooru.db")
\tif err := core.Init(dbPath, types.StrategyFull, false); err != nil {
\t\tt.Fatalf("init db: %v", err)
\t}
\tclient, err := core.New(dbPath, false)
\tif err != nil {
\t\tt.Fatalf("open client: %v", err)
\t}
\tdefer client.Close()

\tuploadDir := filepath.Join(dir, "uploads")
\tcfg := DefaultConfig(filepath.Join(dir, "serve.db"))
\tcfg.Auth.Enabled = false
\tcfg.Encryption.Enabled = true
\tcfg.Encryption.Key = bytes.Repeat([]byte{0x5a}, securekey.Size)
\tcfg.Uploads.Enabled = true
\tcfg.Uploads.Targets = []UploadTarget{{ID: "default", Name: "Default", Path: uploadDir}}
\tserver := NewServerWithLibrary(cfg, NewGooruLibrary(client, false))

\tplaintext := tinyPNG(t, 7, 5, color.RGBA{R: 31, G: 101, B: 211, A: 255})
\trec := httptest.NewRecorder()
\tserver.Handler().ServeHTTP(rec, uploadBinaryRequest(t, map[string][]byte{"secret.png": plaintext}, []string{"private"}))
\tif rec.Code != http.StatusOK {
\t\tt.Fatalf("upload status = %d: %s", rec.Code, rec.Body.String())
\t}

\tstoredPath := filepath.Join(uploadDir, "secret.png")
\tstored, err := os.ReadFile(storedPath)
\tif err != nil {
\t\tt.Fatalf("read stored upload: %v", err)
\t}
\tif bytes.Equal(stored, plaintext) || bytes.Contains(stored, plaintext) {
\t\tt.Fatal("encrypted upload persisted plaintext bytes")
\t}

\topened, err := encryptedfile.Open(storedPath, cfg.Encryption.Key)
\tif err != nil {
\t\tt.Fatalf("open encrypted upload: %v", err)
\t}
\tdecrypted, err := io.ReadAll(opened)
\tif closeErr := opened.Close(); err == nil && closeErr != nil {
\t\terr = closeErr
\t}
\tif err != nil {
\t\tt.Fatalf("read encrypted upload: %v", err)
\t}
\tif !bytes.Equal(decrypted, plaintext) {
\t\tt.Fatal("decrypted upload differs from request body")
\t}

\tfile, err := client.GetFileInfoByPath(storedPath)
\tif err != nil {
\t\tt.Fatalf("uploaded image was not imported: %v", err)
\t}
\tif file.Size != int64(len(plaintext)) {
\t\tt.Fatalf("registered plaintext size = %d, want %d", file.Size, len(plaintext))
\t}
\tmetadata, err := client.GetMediaMetadata(file.ID)
\tif err != nil {
\t\tt.Fatalf("uploaded image metadata: %v", err)
\t}
\tif metadata.ImageWidth == nil || *metadata.ImageWidth != 7 || metadata.ImageHeight == nil || *metadata.ImageHeight != 5 {
\t\tt.Fatalf("uploaded image metadata = %+v", metadata)
\t}

\tcontent := httptest.NewRecorder()
\tserver.Handler().ServeHTTP(content, httptest.NewRequest(http.MethodGet, "/api/v1/files/"+file.PublicID+"/content", nil))
\tif content.Code != http.StatusOK {
\t\tt.Fatalf("content status = %d: %s", content.Code, content.Body.String())
\t}
\tif !bytes.Equal(content.Body.Bytes(), plaintext) {
\t\tt.Fatal("served encrypted upload differs from original plaintext")
\t}
}

func TestEncryptedUploadHonorsPlaintextSizeLimit(t *testing.T) {
\tcfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
\tcfg.Encryption.Enabled = true
\tcfg.Encryption.Key = bytes.Repeat([]byte{0x2c}, securekey.Size)
\tcfg.Uploads.MaxFileSizeBytes = 4
\tserver := NewServer(cfg)

\tdst, err := os.CreateTemp(t.TempDir(), "encrypted-upload-*")
\tif err != nil {
\t\tt.Fatal(err)
\t}
\tdefer dst.Close()
\tif _, err := server.persistUploadedFile(dst, bytes.NewReader([]byte("12345"))); err != errUploadTooLarge {
\t\tt.Fatalf("persist oversized encrypted upload error = %v, want %v", err, errUploadTooLarge)
\t}
}
''')
