from pathlib import Path

Path('internal/hashing/hashes/blake3_full.go').write_text(r'''package hashes

import (
    "encoding/hex"
    "fmt"
    "io"
    "os"

    "github.com/zeebo/blake3"
)

// HashSourceFull computes the full BLAKE3 hash of exactly size bytes from a
// random-access source. Keeping content identity independent of filesystem paths
// lets protected storage expose authenticated plaintext without materializing a
// second plaintext file.
func HashSourceFull(source io.ReaderAt, size int64) (string, error) {
    if source == nil {
        return "", fmt.Errorf("hash source is required")
    }
    if size < 0 {
        return "", fmt.Errorf("hash source size must be non-negative")
    }
    hasher := blake3.New()
    if _, err := io.Copy(hasher, io.NewSectionReader(source, 0, size)); err != nil {
        return "", err
    }
    return hex.EncodeToString(hasher.Sum(nil)), nil
}

// HashFileFull computes the BLAKE3 hash of a file and returns it as a hex string.
func HashFileFull(filePath string) (string, error) {
    file, err := os.Open(filePath)
    if err != nil {
        return "", err
    }
    defer file.Close()
    info, err := file.Stat()
    if err != nil {
        return "", err
    }
    return HashSourceFull(file, info.Size())
}
''')

Path('internal/hashing/hashes/blake3_partial.go').write_text(r'''package hashes

import (
    "encoding/hex"
    "fmt"
    "io"
    "os"

    "github.com/zeebo/blake3"
)

const (
    numChunks = 10
    chunkSize = 64 * 1024
)

const partialHashThreshold = numChunks * chunkSize

// HashSource computes the configured partial-content identity directly from a
// random-access source. The algorithm is byte-for-byte identical to HashFile:
// small files are hashed completely, while large files include size plus ten
// evenly distributed 64 KiB samples.
func HashSource(source io.ReaderAt, size int64) (string, error) {
    if source == nil {
        return "", fmt.Errorf("hash source is required")
    }
    if size < 0 {
        return "", fmt.Errorf("hash source size must be non-negative")
    }
    if size < partialHashThreshold {
        return HashSourceFull(source, size)
    }

    hasher := blake3.New()
    if _, err := fmt.Fprintf(hasher, "%d:", size); err != nil {
        return "", err
    }

    offsets := make([]int64, numChunks)
    if numChunks > 1 {
        span := size - chunkSize
        step := span / int64(numChunks-1)
        for i := int64(0); i < numChunks; i++ {
            offsets[i] = i * step
        }
        offsets[numChunks-1] = size - chunkSize
    } else if numChunks == 1 {
        offsets[0] = (size - chunkSize) / 2
    }

    buf := make([]byte, chunkSize)
    for _, offset := range offsets {
        reader := io.NewSectionReader(source, offset, chunkSize)
        if _, err := io.ReadFull(reader, buf); err != nil {
            return "", fmt.Errorf("failed to read full chunk at offset %d: %w", offset, err)
        }
        if _, err := hasher.Write(buf); err != nil {
            return "", err
        }
    }
    return hex.EncodeToString(hasher.Sum(nil)), nil
}

// HashFile computes a partial hash for large files, or a full hash for small files.
func HashFile(filePath string) (string, error) {
    file, err := os.Open(filePath)
    if err != nil {
        return "", err
    }
    defer file.Close()
    info, err := file.Stat()
    if err != nil {
        return "", err
    }
    return HashSource(file, info.Size())
}
''')

hashing = Path('internal/hashing/hashing.go')
text = hashing.read_text()
text = text.replace('''import (\n\t"fmt"\n\t"runtime"''', '''import (\n\t"fmt"\n\t"io"\n\t"runtime"''')
text = text.replace('''type hashFunc func(string) (string, error)\n\n// Hasher is configured with a specific hashing strategy.\ntype Hasher struct {\n\thashFile hashFunc\n}''', '''type hashFunc func(string) (string, error)\ntype hashSourceFunc func(io.ReaderAt, int64) (string, error)\n\n// Hasher is configured with a specific hashing strategy.\ntype Hasher struct {\n\thashFile   hashFunc\n\thashSource hashSourceFunc\n}''')
text = text.replace('''\tvar hf hashFunc\n\tswitch strategy {\n\tcase types.StrategyPartial:\n\t\thf = hashes.HashFile\n\tcase types.StrategyFull:\n\t\thf = hashes.HashFileFull''', '''\tvar hf hashFunc\n\tvar hs hashSourceFunc\n\tswitch strategy {\n\tcase types.StrategyPartial:\n\t\thf = hashes.HashFile\n\t\ths = hashes.HashSource\n\tcase types.StrategyFull:\n\t\thf = hashes.HashFileFull\n\t\ths = hashes.HashSourceFull''')
text = text.replace('''\treturn &Hasher{hashFile: hf}, nil\n}\n\n// HashFile computes a hash for the given file path using the configured strategy.\nfunc (h *Hasher) HashFile(filePath string) (string, error) {\n\treturn h.hashFile(filePath)\n}''', '''\treturn &Hasher{hashFile: hf, hashSource: hs}, nil\n}\n\n// HashFile computes a hash for the given file path using the configured strategy.\nfunc (h *Hasher) HashFile(filePath string) (string, error) {\n\treturn h.hashFile(filePath)\n}\n\n// HashSource computes the same content identity from an arbitrary random-access\n// source. This is the path-independent primitive used by protected storage.\nfunc (h *Hasher) HashSource(source io.ReaderAt, size int64) (string, error) {\n\treturn h.hashSource(source, size)\n}''')
hashing.write_text(text)

query = Path('gooru/query.go')
text = query.read_text()
text = text.replace('''import (\n\t"database/sql"\n\t"errors"\n\t"fmt"''', '''import (\n\t"database/sql"\n\t"errors"\n\t"fmt"\n\t"io"''')
needle = '''// ContentExists reports whether a content hash is already tracked.\nfunc (c *Client) ContentExists(hash string) (bool, error) {'''
method = '''// GetFileInfoForSource computes content identity and status from a supplied\n// plaintext random-access source while treating filePath as the logical storage\n// location. It mirrors the content-centric branch of GetFileInfoForFile without\n// requiring the logical path itself to contain plaintext bytes.\nfunc (c *Client) GetFileInfoForSource(filePath string, source io.ReaderAt, size int64, modTime int64) (types.FileInfo, types.FileStatus, error) {\n\tabsPath, err := resolvePath(filePath)\n\tif err != nil {\n\t\treturn types.FileInfo{Path: filePath}, 0, err\n\t}\n\tcurrentHash, err := c.hasher.HashSource(source, size)\n\tif err != nil {\n\t\treturn types.FileInfo{Path: filePath}, 0, fmt.Errorf("could not hash file source for verification: %w", err)\n\t}\n\tdbInfo, err := c.store.GetLocationByPath(absPath)\n\tif err != nil && err != sql.ErrNoRows {\n\t\treturn types.FileInfo{Path: filePath}, 0, fmt.Errorf("database lookup failed: %w", err)\n\t}\n\tpathInDB := err != sql.ErrNoRows\n\tif pathInDB {\n\t\tif currentHash != dbInfo.Hash {\n\t\t\treturn types.FileInfo{Path: filePath, Hash: currentHash, Size: size, ModTime: modTime}, types.StatusModified, nil\n\t\t}\n\t\ttags, err := c.store.GetTagsForContent(dbInfo.Hash)\n\t\tif err != nil {\n\t\t\treturn types.FileInfo{Path: filePath}, 0, err\n\t\t}\n\t\treturn types.FileInfo{Path: filePath, Hash: dbInfo.Hash, Size: size, ModTime: modTime, Tags: tags}, types.StatusOK, nil\n\t}\n\ttags, err := c.store.GetTagsForContent(currentHash)\n\tif err != nil {\n\t\treturn types.FileInfo{Path: filePath}, 0, fmt.Errorf("failed to check for existing content: %w", err)\n\t}\n\tinfo := types.FileInfo{Path: filePath, Hash: currentHash, Size: size, ModTime: modTime, Tags: tags}\n\tif len(tags) > 0 {\n\t\treturn info, types.StatusUntrackedContent, nil\n\t}\n\treturn info, types.StatusNotInDB, nil\n}\n\n'''
if needle not in text:
    raise SystemExit('ContentExists insertion point not found')
query.write_text(text.replace(needle, method + needle, 1))

Path('internal/hashing/hashes/hash_source_test.go').write_text(r'''package hashes

import (
    "bytes"
    "os"
    "path/filepath"
    "testing"
)

func TestHashSourceMatchesFileStrategies(t *testing.T) {
    cases := []struct {
        name string
        data []byte
    }{
        {name: "small", data: bytes.Repeat([]byte("small-content-"), 100)},
        {name: "large", data: bytes.Repeat([]byte("large-content-0123456789"), 100000)},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            path := filepath.Join(t.TempDir(), "source.bin")
            if err := os.WriteFile(path, tc.data, 0600); err != nil {
                t.Fatal(err)
            }
            partialFile, err := HashFile(path)
            if err != nil {
                t.Fatal(err)
            }
            partialSource, err := HashSource(bytes.NewReader(tc.data), int64(len(tc.data)))
            if err != nil {
                t.Fatal(err)
            }
            if partialSource != partialFile {
                t.Fatalf("partial source hash = %s, file hash = %s", partialSource, partialFile)
            }
            fullFile, err := HashFileFull(path)
            if err != nil {
                t.Fatal(err)
            }
            fullSource, err := HashSourceFull(bytes.NewReader(tc.data), int64(len(tc.data)))
            if err != nil {
                t.Fatal(err)
            }
            if fullSource != fullFile {
                t.Fatalf("full source hash = %s, file hash = %s", fullSource, fullFile)
            }
        })
    }
}

func TestHashSourceRejectsInvalidSize(t *testing.T) {
    if _, err := HashSource(bytes.NewReader(nil), -1); err == nil {
        t.Fatal("partial source hash should reject negative size")
    }
    if _, err := HashSourceFull(bytes.NewReader(nil), -1); err == nil {
        t.Fatal("full source hash should reject negative size")
    }
}
''')

Path('gooru/file_source_test.go').write_text(r'''package gooru

import (
    "bytes"
    "path/filepath"
    "testing"

    "gooru.local/types"
)

func TestGetFileInfoForSourceMatchesPlaintextContentIdentity(t *testing.T) {
    dir := t.TempDir()
    dbPath := filepath.Join(dir, "gooru.db")
    if err := Init(dbPath, types.StrategyPartial, false); err != nil {
        t.Fatal(err)
    }
    client, err := New(dbPath, false)
    if err != nil {
        t.Fatal(err)
    }
    defer client.Close()

    content := bytes.Repeat([]byte("protected-source-"), 60000)
    logicalPath := filepath.Join(dir, "library", "media.bin")
    info, status, err := client.GetFileInfoForSource(logicalPath, bytes.NewReader(content), int64(len(content)), 1234)
    if err != nil {
        t.Fatal(err)
    }
    if status != types.StatusNotInDB {
        t.Fatalf("status = %v, want StatusNotInDB", status)
    }
    if info.Path != logicalPath || info.Size != int64(len(content)) || info.ModTime != 1234 || info.Hash == "" {
        t.Fatalf("source info = %+v", info)
    }

    if _, err := client.TagKnownFiles([]types.LocationInfo{{
        Path: logicalPath, Hash: info.Hash, Size: info.Size, ModTime: info.ModTime, Extension: filepath.Ext(logicalPath),
    }}, []string{"source:test"}, nil); err != nil {
        t.Fatal(err)
    }
    same, status, err := client.GetFileInfoForSource(logicalPath, bytes.NewReader(content), int64(len(content)), 5678)
    if err != nil {
        t.Fatal(err)
    }
    if status != types.StatusOK || same.Hash != info.Hash || len(same.Tags) != 1 || same.Tags[0] != "source:test" {
        t.Fatalf("same source status/info = %v %+v", status, same)
    }

    changed := append([]byte(nil), content...)
    // StrategyPartial deliberately samples selected regions. Mutate the first
    // sampled chunk so this assertion tests the algorithm rather than assuming
    // unsampled middle bytes affect the configured content identity.
    changed[0] ^= 0xff
    changedInfo, status, err := client.GetFileInfoForSource(logicalPath, bytes.NewReader(changed), int64(len(changed)), 5678)
    if err != nil {
        t.Fatal(err)
    }
    if status != types.StatusModified || changedInfo.Hash == info.Hash {
        t.Fatalf("changed source status/info = %v %+v", status, changedInfo)
    }
}
''')
