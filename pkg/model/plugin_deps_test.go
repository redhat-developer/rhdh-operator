package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadPluginDeps(t *testing.T) {

	dir := t.TempDir()

	// Create subdirectories and files
	// subdir1/
	//  file0.yaml
	//  file1.yaml
	//  subdir11/  - second dir level to always ignore
	//   file11.yaml
	// subdir2/
	//  file2.yaml
	subdir1 := filepath.Join(dir, "subdir1")
	subdir11 := filepath.Join(subdir1, "subdir11")
	subdir2 := filepath.Join(dir, "subdir2")
	err := os.MkdirAll(subdir1, 0755)
	assert.NoError(t, err)
	err = os.MkdirAll(subdir2, 0755)
	assert.NoError(t, err)
	err = os.MkdirAll(subdir11, 0755)
	assert.NoError(t, err)

	file0 := filepath.Join(subdir1, "file0.yaml")
	file1 := filepath.Join(subdir1, "file1.yaml")
	file11 := filepath.Join(subdir11, "file11.yaml")
	file2 := filepath.Join(subdir2, "file2.yaml")
	err = os.WriteFile(file1, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test1"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(file2, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test2"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(file11, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test11"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(file0, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test0"), 0644)
	assert.NoError(t, err)

	// Call ReadPluginDeps for subdir1
	objects, err := ReadPluginDeps(dir, "", "", []string{"subdir1"})
	assert.NoError(t, err)
	assert.Len(t, objects, 2)
	assert.Equal(t, "test0", objects[0].GetName())
	assert.Equal(t, "test1", objects[1].GetName())

	objects, err = ReadPluginDeps(dir, "", "", []string{""})
	assert.NoError(t, err)
	assert.Len(t, objects, 0)
}

func TestReadPluginDepsSubstitutions(t *testing.T) {

	dir := t.TempDir()

	// Create subdirectory and a YAML file with placeholders
	subdir1 := filepath.Join(dir, "subdir1")
	err := os.MkdirAll(subdir1, 0755)
	assert.NoError(t, err)

	file1 := filepath.Join(subdir1, "file1.yaml")
	yamlContent := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{backstage-name}}
  namespace: {{backstage-ns}}
`
	err = os.WriteFile(file1, []byte(yamlContent), 0644)
	assert.NoError(t, err)

	// Call ReadPluginDeps with substitution values
	bsName := "test-name"
	bsNamespace := "test-namespace"
	objects, err := ReadPluginDeps(dir, bsName, bsNamespace, []string{"subdir1"})
	assert.NoError(t, err)
	assert.Len(t, objects, 1)

	// Verify the substitutions
	assert.Equal(t, "test-name", objects[0].GetName())
	assert.Equal(t, "test-namespace", objects[0].GetNamespace())
}
