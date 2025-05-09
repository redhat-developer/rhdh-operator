package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadPluginDeps(t *testing.T) {
	dir := t.TempDir()

	// Create files in the root directory
	file1 := filepath.Join(dir, "sonata.yaml")
	file2 := filepath.Join(dir, "otherplugin.yaml")
	file3 := filepath.Join(dir, "sonata-config.yaml")
	file4 := filepath.Join(dir, "unrelated.txt")

	err := os.WriteFile(file1, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: sonata"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(file2, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test2"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(file3, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: sonata"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(file4, []byte("some unrelated content"), 0644)
	assert.NoError(t, err)

	objects, err := ReadPluginDeps(dir, "", "", []string{"sonata"})
	assert.NoError(t, err)
	assert.Len(t, objects, 2)

	// Verify the names of the objects
	assert.Equal(t, "sonata", objects[0].GetName())
	assert.Equal(t, "sonata", objects[1].GetName())
}

func TestReadPluginDepsSubstitutions(t *testing.T) {

	dir := t.TempDir()

	file1 := filepath.Join(dir, "file1.yaml")
	yamlContent := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{backstage-name}}
  namespace: {{backstage-ns}}
`
	err := os.WriteFile(file1, []byte(yamlContent), 0644)
	assert.NoError(t, err)

	// Call ReadPluginDeps with substitution values
	bsName := "test-name"
	bsNamespace := "test-namespace"
	objects, err := ReadPluginDeps(dir, bsName, bsNamespace, []string{"file1"})
	assert.NoError(t, err)
	assert.Len(t, objects, 1)

	// Verify the substitutions
	assert.Equal(t, "test-name", objects[0].GetName())
	assert.Equal(t, "test-namespace", objects[0].GetNamespace())
}
