package fetcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Tests: copyLocal()
// ============================================================================

func TestCopyLocal_SingleFile(t *testing.T) {
	// Create source file
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "test.txt")
	content := []byte("hello world")
	require.NoError(t, os.WriteFile(srcFile, content, 0644))

	// Copy to destination
	destDir := t.TempDir()
	err := copyLocal(srcFile, destDir)
	require.NoError(t, err)

	// Verify file was copied
	data, err := os.ReadFile(filepath.Join(destDir, "test.txt"))
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestCopyLocal_Directory(t *testing.T) {
	// Create source directory with files
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file2.txt"), []byte("content2"), 0644))

	// Create subdirectory with file
	subDir := filepath.Join(srcDir, "subdir")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte("nested"), 0644))

	// Copy to destination
	destDir := t.TempDir()
	err := copyLocal(srcDir, destDir)
	require.NoError(t, err)

	// Verify files were copied
	data, err := os.ReadFile(filepath.Join(destDir, "file1.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content1", string(data))

	data, err = os.ReadFile(filepath.Join(destDir, "file2.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content2", string(data))

	data, err = os.ReadFile(filepath.Join(destDir, "subdir", "nested.txt"))
	require.NoError(t, err)
	assert.Equal(t, "nested", string(data))
}

func TestCopyLocal_NonexistentSource(t *testing.T) {
	destDir := t.TempDir()
	err := copyLocal("/nonexistent/path/file.txt", destDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to stat")
}

func TestCopyLocal_CreatesDestDir(t *testing.T) {
	// Create source file
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "test.txt")
	require.NoError(t, os.WriteFile(srcFile, []byte("content"), 0644))

	// Use non-existent destination directory
	destDir := filepath.Join(t.TempDir(), "new", "nested", "dir")

	err := copyLocal(srcFile, destDir)
	require.NoError(t, err)

	// Verify destination was created and file copied
	assert.FileExists(t, filepath.Join(destDir, "test.txt"))
}

// ============================================================================
// Tests: copyFile()
// ============================================================================

func TestCopyFile(t *testing.T) {
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "source.txt")
	content := []byte("test content")
	require.NoError(t, os.WriteFile(srcFile, content, 0644))

	destDir := t.TempDir()
	destFile := filepath.Join(destDir, "dest.txt")

	err := copyFile(srcFile, destFile)
	require.NoError(t, err)

	data, err := os.ReadFile(destFile)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestCopyFile_LargeFile(t *testing.T) {
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "large.bin")

	// Create a 1MB file
	content := make([]byte, 1024*1024)
	for i := range content {
		content[i] = byte(i % 256)
	}
	require.NoError(t, os.WriteFile(srcFile, content, 0644))

	destDir := t.TempDir()
	destFile := filepath.Join(destDir, "large.bin")

	err := copyFile(srcFile, destFile)
	require.NoError(t, err)

	data, err := os.ReadFile(destFile)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

// ============================================================================
// Tests: copyDir()
// ============================================================================

func TestCopyDir_Empty(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()

	err := copyDir(srcDir, destDir)
	require.NoError(t, err)

	// Destination should exist but be empty
	entries, err := os.ReadDir(destDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestCopyDir_DeepNesting(t *testing.T) {
	srcDir := t.TempDir()

	// Create deeply nested structure
	deepPath := filepath.Join(srcDir, "a", "b", "c", "d")
	require.NoError(t, os.MkdirAll(deepPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(deepPath, "deep.txt"), []byte("deep"), 0644))

	destDir := t.TempDir()
	err := copyDir(srcDir, destDir)
	require.NoError(t, err)

	// Verify deep file was copied
	data, err := os.ReadFile(filepath.Join(destDir, "a", "b", "c", "d", "deep.txt"))
	require.NoError(t, err)
	assert.Equal(t, "deep", string(data))
}

// ============================================================================
// Tests: Fetcher with file: protocol
// ============================================================================

func TestFetcher_FileProtocol_AbsolutePath(t *testing.T) {
	// Create source directory with plugin files
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "package.json"), []byte(`{"name": "test-plugin"}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "index.js"), []byte("module.exports = {};"), 0644))

	f := New()
	destDir := t.TempDir()

	// Test file:/path format
	err := f.Fetch(context.Background(), "file:"+srcDir, destDir)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(destDir, "package.json"))
	assert.FileExists(t, filepath.Join(destDir, "index.js"))
}

func TestFetcher_FileProtocol_TripleSlash(t *testing.T) {
	// Create source directory
	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "plugin.js"), []byte("// plugin"), 0644))

	f := New()
	destDir := t.TempDir()

	// Test file:///path format (empty authority)
	// After trimming "file:", we get "//path" which needs to be handled
	err := f.Fetch(context.Background(), "file://"+srcDir, destDir)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(destDir, "plugin.js"))
}

func TestFetcher_FileProtocol_RelativePath(t *testing.T) {
	// Create source directory in current working directory
	srcDir, err := os.MkdirTemp(".", "test-plugin-*")
	require.NoError(t, err)
	defer os.RemoveAll(srcDir)

	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "package.json"), []byte(`{"name": "rel-plugin"}`), 0644))

	f := New()
	destDir := t.TempDir()

	// Test file:./path format
	err = f.Fetch(context.Background(), "file:"+srcDir, destDir)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(destDir, "package.json"))
}

func TestFetcher_FileProtocol_SingleFile(t *testing.T) {
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "plugin.tgz")
	require.NoError(t, os.WriteFile(srcFile, []byte("tarball content"), 0644))

	f := New()
	destDir := t.TempDir()

	err := f.Fetch(context.Background(), "file:"+srcFile, destDir)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(destDir, "plugin.tgz"))
}

func TestFetcher_FileProtocol_NonexistentPath(t *testing.T) {
	f := New()
	destDir := t.TempDir()

	err := f.Fetch(context.Background(), "file:/nonexistent/path", destDir)
	assert.Error(t, err)
}
