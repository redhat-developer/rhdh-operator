package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/redhat-developer/rhdh-operator/api"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
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

	objects, err := ReadPluginDeps(dir, "", "", []string{"sonata"}, "")
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
	objects, err := ReadPluginDeps(dir, bsName, bsNamespace, []string{"file1"}, "")
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
	bs := api.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-name",
			Namespace: "test-namespace",
		},
	}
	sc := runtime.NewScheme()
	utilruntime.Must(api.AddToScheme(sc))
	objects, err := GetPluginDeps(bs, dynaPlugins, sc, "")
	assert.NoError(t, err)
	assert.Len(t, objects, 2)

	// Verify the returned objects
	actualNames := []string{objects[0].GetName(), objects[1].GetName()}
	expectedNames := []string{"dep1", "dep2"}
	assert.ElementsMatch(t, expectedNames, actualNames)

	// Verify ownerReference
	for _, obj := range objects {
		ownerRefs := obj.GetOwnerReferences()
		assert.Len(t, ownerRefs, 1)
		assert.Equal(t, bs.Name, ownerRefs[0].Name)
		assert.Equal(t, "Backstage", ownerRefs[0].Kind)
	}
}

func TestReadPluginDepsNoFiles(t *testing.T) {
	dir := t.TempDir()

	// Call ReadPluginDeps with an empty directory
	objects, err := ReadPluginDeps(dir, "", "", []string{"sonata"}, "")
	assert.NoError(t, err)
	assert.Len(t, objects, 0)
}

func TestReadPluginDepsPlatformGating(t *testing.T) {
	dir := t.TempDir()

	// Create files with different platform annotations
	ocpOnly := filepath.Join(dir, "plugin-ocp.yaml")
	k8sOnly := filepath.Join(dir, "plugin-k8s.yaml")
	allPlatforms := filepath.Join(dir, "plugin-all.yaml")

	// OCP-only resource
	err := os.WriteFile(ocpOnly, []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: ocp-config
  annotations:
    rhdh.redhat.com/platform: ocp
`), 0644)
	assert.NoError(t, err)

	// K8s-only resource
	err = os.WriteFile(k8sOnly, []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: k8s-config
  annotations:
    rhdh.redhat.com/platform: k8s
`), 0644)
	assert.NoError(t, err)

	// All platforms (no annotation)
	err = os.WriteFile(allPlatforms, []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: all-config
`), 0644)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		platformExt string
		wantNames   []string
	}{
		{
			name:        "ocp platform gets ocp and all",
			platformExt: "ocp",
			wantNames:   []string{"ocp-config", "all-config"},
		},
		{
			name:        "k8s platform gets k8s and all",
			platformExt: "k8s",
			wantNames:   []string{"k8s-config", "all-config"},
		},
		{
			name:        "empty platform gets all resources",
			platformExt: "",
			wantNames:   []string{"ocp-config", "k8s-config", "all-config"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects, err := ReadPluginDeps(dir, "", "", []string{"plugin"}, tt.platformExt)
			assert.NoError(t, err)

			var gotNames []string
			for _, obj := range objects {
				gotNames = append(gotNames, obj.GetName())
			}
			assert.ElementsMatch(t, tt.wantNames, gotNames)
		})
	}
}

func TestMatchesPlatform(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		platformExt string
		want        bool
	}{
		{
			name:        "no annotations - matches all",
			annotations: nil,
			platformExt: "ocp",
			want:        true,
		},
		{
			name:        "no platform annotation - matches all",
			annotations: map[string]string{"other": "value"},
			platformExt: "ocp",
			want:        true,
		},
		{
			name:        "ocp annotation matches ocp platform",
			annotations: map[string]string{PlatformAnnotation: "ocp"},
			platformExt: "ocp",
			want:        true,
		},
		{
			name:        "k8s annotation matches k8s platform",
			annotations: map[string]string{PlatformAnnotation: "k8s"},
			platformExt: "k8s",
			want:        true,
		},
		{
			name:        "ocp annotation does not match k8s platform",
			annotations: map[string]string{PlatformAnnotation: "ocp"},
			platformExt: "k8s",
			want:        false,
		},
		{
			name:        "k8s annotation does not match ocp platform",
			annotations: map[string]string{PlatformAnnotation: "k8s"},
			platformExt: "ocp",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{}
			if tt.annotations != nil {
				obj.SetAnnotations(tt.annotations)
			}
			got := matchesPlatform(obj, tt.platformExt)
			assert.Equal(t, tt.want, got)
		})
	}
}
