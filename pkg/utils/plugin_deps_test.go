package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadPluginDeps(t *testing.T) {
	// Create a temporary directory
	dir := t.TempDir()

	// Create a mock "enabled" file
	enabledContent := filepath.Clean("subdir1")
	err := os.WriteFile(filepath.Join(dir, "enabled"), []byte(enabledContent), 0644)
	assert.NoError(t, err)

	// Create subdirectories and files
	subdir1 := filepath.Join(dir, "subdir1")
	subdir11 := filepath.Join(subdir1, "subdir11")
	subdir2 := filepath.Join(dir, "subdir2")
	err = os.MkdirAll(subdir1, 0755)
	assert.NoError(t, err)
	err = os.MkdirAll(subdir2, 0755)
	assert.NoError(t, err)
	err = os.MkdirAll(subdir11, 0755)
	assert.NoError(t, err)

	file0 := filepath.Join(dir, "file0.yaml")
	file1 := filepath.Join(subdir1, "file1.yaml")
	file2 := filepath.Join(subdir2, "file2.yaml")
	file11 := filepath.Join(subdir11, "file2.yaml")
	err = os.WriteFile(file1, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test1"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(file2, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test2"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(file11, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test11"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(file0, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test11"), 0644)
	assert.NoError(t, err)

	// Call ReadPluginDeps
	objects, err := ReadPluginDeps(dir)
	assert.NoError(t, err)
	assert.Len(t, objects, 2)
	assert.Equal(t, "test1", objects[0].GetName())
	assert.Equal(t, "test11", objects[1].GetName())

	enabledContent = filepath.Clean("")
	err = os.WriteFile(filepath.Join(dir, "enabled"), []byte(enabledContent), 0644)
	objects, err = ReadPluginDeps(dir)
	assert.NoError(t, err)
	assert.Len(t, objects, 4)

}

func TestReadEnabledDirs(t *testing.T) {
	// Create a temporary file
	dir := t.TempDir()
	enabledFile := filepath.Join(dir, "enabled")

	// Write content to the file
	content := "subdir1\nsubdir2\n"
	err := os.WriteFile(enabledFile, []byte(content), 0644)
	assert.NoError(t, err)

	// Call readEnabledDirs
	enabledDirs, err := readEnabledDirs(enabledFile)
	root := filepath.Dir(enabledFile)
	assert.NoError(t, err)
	assert.Len(t, enabledDirs, 2)
	assert.Contains(t, enabledDirs, filepath.Join(root, "subdir1"))
	assert.Contains(t, enabledDirs, filepath.Join(root, "subdir2"))
}
