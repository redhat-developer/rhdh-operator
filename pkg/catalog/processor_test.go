package catalog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/redhat-developer/rhdh-operator/pkg/model"
)

func TestNewProcessor(t *testing.T) {
	p := NewProcessor()
	assert.NotNil(t, p)
}

func TestMerge_SingleCatalog(t *testing.T) {
	p := NewProcessor()

	catalog := `plugins:
  - package: "oci://registry.example.com/rhdh/plugin-techdocs:1.0"
    disabled: false
  - package: "oci://registry.example.com/rhdh/plugin-kubernetes:1.0"
    disabled: false
`

	result, err := p.merge([][]byte{[]byte(catalog)})
	require.NoError(t, err)

	var merged model.DynaPluginsConfig
	require.NoError(t, yaml.Unmarshal(result, &merged))

	assert.Len(t, merged.Plugins, 2)
}

func TestMerge_MultipleCatalogs(t *testing.T) {
	p := NewProcessor()

	catalog1 := `plugins:
  - package: "oci://registry.example.com/rhdh/plugin-techdocs:1.0"
    disabled: false
  - package: "oci://registry.example.com/rhdh/plugin-kubernetes:1.0"
    disabled: false
`

	catalog2 := `plugins:
  - package: "oci://registry.example.com/rhdh/plugin-argocd:1.0"
    disabled: false
  - package: "oci://registry.example.com/rhdh/plugin-search:1.0"
    disabled: false
`

	result, err := p.merge([][]byte{[]byte(catalog1), []byte(catalog2)})
	require.NoError(t, err)

	var merged model.DynaPluginsConfig
	require.NoError(t, yaml.Unmarshal(result, &merged))

	assert.Len(t, merged.Plugins, 4)
}

func TestMerge_DuplicateSameURL_Fails(t *testing.T) {
	p := NewProcessor()

	catalog := `plugins:
  - package: "oci://registry.example.com/rhdh/plugin-techdocs:1.0"
    disabled: false
  - package: "oci://registry.example.com/rhdh/plugin-techdocs:1.0"
    disabled: true
`

	_, err := p.merge([][]byte{[]byte(catalog)})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate plugin name")
	assert.Contains(t, err.Error(), "plugin-techdocs")
}

func TestMerge_DuplicateDifferentURL_Fails(t *testing.T) {
	p := NewProcessor()

	// Same plugin name but different registries/tags
	catalog := `plugins:
  - package: "oci://registry.example.com/rhdh/plugin-techdocs:1.0"
    disabled: false
  - package: "oci://other-registry.com/path/plugin-techdocs@sha256:abc123"
    disabled: false
`

	_, err := p.merge([][]byte{[]byte(catalog)})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate plugin name")
	assert.Contains(t, err.Error(), "plugin-techdocs")
}

func TestMerge_DuplicateAcrossCatalogs_Fails(t *testing.T) {
	p := NewProcessor()

	catalog1 := `plugins:
  - package: "oci://registry.example.com/rhdh/plugin-techdocs:1.0"
    disabled: false
`

	catalog2 := `plugins:
  - package: "oci://other-registry.com/rhdh/plugin-techdocs:2.0"
    disabled: false
`

	_, err := p.merge([][]byte{[]byte(catalog1), []byte(catalog2)})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate plugin name")
	assert.Contains(t, err.Error(), "plugin-techdocs")
}

func TestMerge_EmptyCatalog(t *testing.T) {
	p := NewProcessor()

	catalog := `plugins: []`

	result, err := p.merge([][]byte{[]byte(catalog)})
	require.NoError(t, err)

	var merged model.DynaPluginsConfig
	require.NoError(t, yaml.Unmarshal(result, &merged))

	assert.Empty(t, merged.Plugins)
}

func TestMerge_InvalidYAML(t *testing.T) {
	p := NewProcessor()

	invalidYAML := `this is not: valid: yaml: [}`

	_, err := p.merge([][]byte{[]byte(invalidYAML)})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse catalog")
}

func TestMerge_InvalidPluginPackage(t *testing.T) {
	p := NewProcessor()

	// Plugin with unsupported URL format that results in empty name
	catalog := `plugins:
  - package: "invalid-format"
    disabled: false
`

	_, err := p.merge([][]byte{[]byte(catalog)})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot extract plugin name")
}

func TestMerge_PreservesPluginConfig(t *testing.T) {
	p := NewProcessor()

	catalog := `plugins:
  - package: "oci://registry.example.com/rhdh/plugin-github:1.0"
    disabled: false
    pluginConfig:
      catalog:
        providers:
          github:
            organization: my-org
`

	result, err := p.merge([][]byte{[]byte(catalog)})
	require.NoError(t, err)

	var merged model.DynaPluginsConfig
	require.NoError(t, yaml.Unmarshal(result, &merged))

	assert.Len(t, merged.Plugins, 1)
	assert.NotNil(t, merged.Plugins[0].PluginConfig)
}

func TestWrapInConfigMap(t *testing.T) {
	p := NewProcessor()

	content := []byte(`plugins:
  - package: "oci://registry.example.com/rhdh/plugin-test:1.0"
`)

	wrapped := p.wrapInConfigMap(content)
	wrappedStr := string(wrapped)

	// Verify it's valid YAML
	var cm map[string]interface{}
	require.NoError(t, yaml.Unmarshal(wrapped, &cm))

	// Verify structure
	assert.Equal(t, "v1", cm["apiVersion"])
	assert.Equal(t, "ConfigMap", cm["kind"])

	metadata, ok := cm["metadata"].(map[interface{}]interface{})
	require.True(t, ok)
	assert.Equal(t, InternalConfigMapName, metadata["name"])

	// Verify data key
	data, ok := cm["data"].(map[interface{}]interface{})
	require.True(t, ok)
	assert.Contains(t, data, model.DynamicPluginsFile)

	// The content should contain our plugin
	assert.Contains(t, wrappedStr, "oci://registry.example.com/rhdh/plugin-test:1.0")
}

func TestIndentYAML(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		spaces   int
		expected string
	}{
		{
			name:     "simple indent",
			content:  "line1\nline2",
			spaces:   2,
			expected: "  line1\n  line2",
		},
		{
			name:     "four spaces",
			content:  "key: value",
			spaces:   4,
			expected: "    key: value",
		},
		{
			name:     "empty lines preserved",
			content:  "line1\n\nline3",
			spaces:   2,
			expected: "  line1\n\n  line3",
		},
		{
			name:     "zero spaces",
			content:  "no indent",
			spaces:   0,
			expected: "no indent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := indentYAML(tt.content, tt.spaces)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCatalogInput(t *testing.T) {
	input := CatalogInput{
		Ref:           "oci://registry.example.com/rhdh/plugin-catalog-index:v1",
		DockerConfig:  []byte(`{"auths":{}}`),
		CACert:        []byte("-----BEGIN CERTIFICATE-----"),
		SkipTLSVerify: true,
	}

	assert.Equal(t, "oci://registry.example.com/rhdh/plugin-catalog-index:v1", input.Ref)
	assert.NotNil(t, input.DockerConfig)
	assert.NotNil(t, input.CACert)
	assert.True(t, input.SkipTLSVerify)
}

func TestConstants(t *testing.T) {
	assert.Equal(t, "catalog-dynamic-plugins", InternalConfigMapName)
	assert.Equal(t, "dynamic-plugins.default.yaml", CatalogFileName)
	assert.Equal(t, ".catalogs-ready", CatalogsReadyKey)
}

func TestProcessOutputFormat(t *testing.T) {
	// Verifies the JSON patch structure without actually fetching

	patchData := map[string]interface{}{
		"data": map[string]string{
			model.DynamicPluginsFile: "test content",
			CatalogsReadyKey:         "true",
		},
	}

	jsonBytes, err := json.Marshal(patchData)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))

	data, ok := parsed["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "test content", data[model.DynamicPluginsFile])
	assert.Equal(t, "true", data[CatalogsReadyKey])
}

func TestWrapInConfigMap_MultilineContent(t *testing.T) {
	p := NewProcessor()

	content := []byte(`plugins:
  - package: "oci://registry.example.com/rhdh/plugin-github:1.0"
    disabled: false
    pluginConfig:
      catalog:
        providers:
          github:
            organization: my-org
`)

	wrapped := p.wrapInConfigMap(content)

	// Should produce valid YAML
	var cm map[string]interface{}
	require.NoError(t, yaml.Unmarshal(wrapped, &cm))

	wrappedStr := string(wrapped)
	assert.True(t, strings.Contains(wrappedStr, "data:"))
	assert.True(t, strings.Contains(wrappedStr, model.DynamicPluginsFile+": |"))
}
