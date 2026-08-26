package fetcher

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractTarBytes(t *testing.T) {
	// Create a tar archive in memory
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Add a file
	content := []byte("hello world")
	hdr := &tar.Header{
		Name: "test.txt",
		Mode: 0644,
		Size: int64(len(content)),
	}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err := tw.Write(content)
	require.NoError(t, err)

	// Add a directory
	dirHdr := &tar.Header{
		Name:     "subdir/",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}
	require.NoError(t, tw.WriteHeader(dirHdr))

	// Add a file in subdirectory
	subContent := []byte("nested content")
	subHdr := &tar.Header{
		Name: "subdir/nested.txt",
		Mode: 0644,
		Size: int64(len(subContent)),
	}
	require.NoError(t, tw.WriteHeader(subHdr))
	_, err = tw.Write(subContent)
	require.NoError(t, err)

	require.NoError(t, tw.Close())

	// Extract to temp directory
	destDir := t.TempDir()
	err = extractTarBytes(buf.Bytes(), destDir)
	require.NoError(t, err)

	// Verify files exist
	data, err := os.ReadFile(filepath.Join(destDir, "test.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(data))

	data, err = os.ReadFile(filepath.Join(destDir, "subdir", "nested.txt"))
	require.NoError(t, err)
	assert.Equal(t, "nested content", string(data))
}

func TestExtractTarGzBytes(t *testing.T) {
	// Create a gzipped tar archive
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	content := []byte("gzipped content")
	hdr := &tar.Header{
		Name: "gzipped.txt",
		Mode: 0644,
		Size: int64(len(content)),
	}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err := tw.Write(content)
	require.NoError(t, err)

	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())

	// Extract
	destDir := t.TempDir()
	err = extractTarGzBytes(buf.Bytes(), destDir)
	require.NoError(t, err)

	// Verify
	data, err := os.ReadFile(filepath.Join(destDir, "gzipped.txt"))
	require.NoError(t, err)
	assert.Equal(t, "gzipped content", string(data))
}

func TestExtractTarPathTraversal(t *testing.T) {
	// Create a tar with path traversal attempt
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	hdr := &tar.Header{
		Name: "../../../etc/passwd",
		Mode: 0644,
		Size: 0,
	}
	require.NoError(t, tw.WriteHeader(hdr))
	require.NoError(t, tw.Close())

	destDir := t.TempDir()
	err := extractTarBytes(buf.Bytes(), destDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tar path")
}

func TestExtractTarSymlinkEscape(t *testing.T) {
	// Create a tar with symlink escape attempt
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	hdr := &tar.Header{
		Name:     "escape-link",
		Mode:     0777,
		Typeflag: tar.TypeSymlink,
		Linkname: "../../../etc/passwd",
	}
	require.NoError(t, tw.WriteHeader(hdr))
	require.NoError(t, tw.Close())

	destDir := t.TempDir()
	err := extractTarBytes(buf.Bytes(), destDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "symlink escapes destination")
}

func TestExtractTarValidSymlink(t *testing.T) {
	// Create a tar with valid internal symlink
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Add target file
	content := []byte("target content")
	hdr := &tar.Header{
		Name: "target.txt",
		Mode: 0644,
		Size: int64(len(content)),
	}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err := tw.Write(content)
	require.NoError(t, err)

	// Add symlink to target
	linkHdr := &tar.Header{
		Name:     "link.txt",
		Mode:     0777,
		Typeflag: tar.TypeSymlink,
		Linkname: "target.txt",
	}
	require.NoError(t, tw.WriteHeader(linkHdr))
	require.NoError(t, tw.Close())

	destDir := t.TempDir()
	err = extractTarBytes(buf.Bytes(), destDir)
	require.NoError(t, err)

	// Verify symlink works
	data, err := os.ReadFile(filepath.Join(destDir, "link.txt"))
	require.NoError(t, err)
	assert.Equal(t, "target content", string(data))
}

func TestExtractTarEmptyArchive(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.Close())

	destDir := t.TempDir()
	err := extractTarBytes(buf.Bytes(), destDir)
	assert.NoError(t, err)
}

func TestGetMaxEntrySize(t *testing.T) {
	// Test default value (env not set)
	assert.Equal(t, int64(DefaultMaxEntrySize), getMaxEntrySize())

	// Test custom value
	t.Setenv("MAX_ENTRY_SIZE", "1000")
	assert.Equal(t, int64(1000), getMaxEntrySize())

	// Test invalid value falls back to default
	t.Setenv("MAX_ENTRY_SIZE", "invalid")
	assert.Equal(t, int64(DefaultMaxEntrySize), getMaxEntrySize())

	// Test negative value falls back to default
	t.Setenv("MAX_ENTRY_SIZE", "-100")
	assert.Equal(t, int64(DefaultMaxEntrySize), getMaxEntrySize())
}

func TestExtractTarHeaderSizeCheck(t *testing.T) {
	// Set a small max size for testing
	t.Setenv("MAX_ENTRY_SIZE", "100")

	// Create a tar with a file larger than the limit
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Create content larger than MAX_ENTRY_SIZE
	content := make([]byte, 200)
	for i := range content {
		content[i] = 'x'
	}

	hdr := &tar.Header{
		Name: "large-file.txt",
		Mode: 0644,
		Size: int64(len(content)), // 200 bytes exceeds MAX_ENTRY_SIZE of 100
	}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err := tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	destDir := t.TempDir()
	err = extractTarBytes(buf.Bytes(), destDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum size")
}

func TestDefaultMaxEntrySize(t *testing.T) {
	// Verify the default is 40MB
	assert.Equal(t, int64(40_000_000), int64(DefaultMaxEntrySize))
}
