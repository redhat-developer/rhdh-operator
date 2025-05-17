package model

import (
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	bsName := "test-name"
	bsNamespace := "test-namespace"
	objects, err := ReadPluginDeps(dir, bsName, bsNamespace, []string{"file1"})
	assert.NoError(t, err)
	assert.Len(t, objects, 1)

	assert.Equal(t, bsName, objects[0].GetName())
	assert.Equal(t, bsNamespace, objects[0].GetNamespace())
}

func TestGetPluginDeps(t *testing.T) {
	// Setup temporary directory
	tempDir := t.TempDir()
	t.Setenv("LOCALBIN", tempDir)

	pluginDepsDir := filepath.Join(tempDir, "plugin-deps")
	err := os.Mkdir(pluginDepsDir, 0755)
	assert.NoError(t, err)

	// Create mock plugin dependency files
	file1 := filepath.Join(pluginDepsDir, "dep1.yaml")
	file2 := filepath.Join(pluginDepsDir, "dep2.yaml")

	err = os.WriteFile(file1, []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: dep1
`), 0644)
	assert.NoError(t, err)

	err = os.WriteFile(file2, []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: dep2
`), 0644)
	assert.NoError(t, err)

	dynaPlugins := DynamicPlugins{
		ConfigMap: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-configmap",
			},
			Data: map[string]string{
				"dynamic-plugins.yaml": `
includes:
  - dynamic-plugins.default.yaml
plugins:
  - package: './dynamic-plugins/dist/test'
    disabled: false
    dependencies:
      - ref: dep1
      - ref: dep2
`,
			},
		},
	}

	// Call GetPluginDeps
	bsName := "test-name"
	bsNamespace := "test-namespace"
	objects, err := GetPluginDeps(bsName, bsNamespace, dynaPlugins)
	assert.NoError(t, err)
	assert.Len(t, objects, 2)

	// Verify the returned objects
	actualNames := []string{objects[0].GetName(), objects[1].GetName()}
	expectedNames := []string{"dep1", "dep2"}
	assert.ElementsMatch(t, expectedNames, actualNames)
}

func TestReadPluginDepsNoFiles(t *testing.T) {
	dir := t.TempDir()

	// Call ReadPluginDeps with an empty directory
	objects, err := ReadPluginDeps(dir, "", "", []string{"sonata"})
	assert.NoError(t, err)
	assert.Len(t, objects, 0)
}
