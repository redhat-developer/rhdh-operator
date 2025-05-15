package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadYamlFile(t *testing.T) {
	// Create a temporary directory
	dir, err := os.MkdirTemp("", "test-yaml")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	// Create a sample YAML file
	yamlContent := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap
data:
  key: value
`
	filePath := filepath.Join(dir, "test.yaml")
	err = os.WriteFile(filePath, []byte(yamlContent), 0644)
	assert.NoError(t, err)

	// Test ReadYamlFile
	objects, err := ReadYamlFile(filePath)
	assert.NoError(t, err)
	assert.Len(t, objects, 1)

	obj := objects[0]
	assert.Equal(t, "ConfigMap", obj.GetKind())
	assert.Equal(t, "test-configmap", obj.GetName())
}

func TestReadYamlFilesFromDir(t *testing.T) {
	// Create a temporary directory
	dir, err := os.MkdirTemp("", "test-yaml-dir")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	// Create sample YAML files
	yamlContent1 := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap1
data:
  key: value1
`
	yamlContent2 := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap2
data:
  key: value2
`
	err = os.WriteFile(filepath.Join(dir, "test1.yaml"), []byte(yamlContent1), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "test2.yaml"), []byte(yamlContent2), 0644)
	assert.NoError(t, err)

	// Test ReadYamlFilesFromDir
	objects, err := ReadYamlFilesFromDir(dir)
	assert.NoError(t, err)
	assert.Len(t, objects, 2)

	obj1 := objects[0]
	assert.Equal(t, "ConfigMap", obj1.GetKind())
	assert.Equal(t, "test-configmap1", obj1.GetName())

	obj2 := objects[1]
	assert.Equal(t, "ConfigMap", obj2.GetKind())
	assert.Equal(t, "test-configmap2", obj2.GetName())
}
