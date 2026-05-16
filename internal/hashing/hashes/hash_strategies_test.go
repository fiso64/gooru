package hashes

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpecHashingStrategies_PartialSmallFilesFallbackToFullHash(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "small.bin")
	require.NoError(t, os.WriteFile(file, []byte("small content"), 0644))

	partial, err := HashFile(file)
	require.NoError(t, err)
	full, err := HashFileFull(file)
	require.NoError(t, err)

	assert.Equal(t, full, partial)
}

func TestSpecHashingStrategies_PartialLargeHashIncludesFileSize(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.bin")
	b := filepath.Join(dir, "b.bin")
	require.NoError(t, os.WriteFile(a, bytes.Repeat([]byte{0}, partialHashThreshold+1), 0644))
	require.NoError(t, os.WriteFile(b, bytes.Repeat([]byte{0}, partialHashThreshold+2), 0644))

	hashA, err := HashFile(a)
	require.NoError(t, err)
	hashB, err := HashFile(b)
	require.NoError(t, err)

	assert.NotEqual(t, hashA, hashB)
}

func TestSpecHashingStrategies_PartialLargeHashSamplesChunksOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.bin")
	b := filepath.Join(dir, "b.bin")
	contentA := bytes.Repeat([]byte{0}, partialHashThreshold*2)
	contentB := bytes.Clone(contentA)
	contentB[chunkSize+1] = 1
	require.NoError(t, os.WriteFile(a, contentA, 0644))
	require.NoError(t, os.WriteFile(b, contentB, 0644))

	partialA, err := HashFile(a)
	require.NoError(t, err)
	partialB, err := HashFile(b)
	require.NoError(t, err)
	fullA, err := HashFileFull(a)
	require.NoError(t, err)
	fullB, err := HashFileFull(b)
	require.NoError(t, err)

	assert.Equal(t, partialA, partialB)
	assert.NotEqual(t, fullA, fullB)
}
