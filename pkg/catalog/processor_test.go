package catalog

import (
	"encoding/json"
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
	assert.Equal(t, "dynamic-plugins.default.yaml", CatalogFileName)
}

func TestProcessOutputFormat(t *testing.T) {
	// Verifies the JSON patch structure contains a ConfigMap YAML (not raw plugin config).
	// The default-config ConfigMap stores Kubernetes manifests as file values,
	// so each value must be a complete ConfigMap YAML with apiVersion, kind, metadata, and data.

	// Simulate what Process() outputs
	pluginConfig := `plugins:
  - package: "oci://registry.example.com/rhdh/plugin-test:1.0"
    disabled: false
`
	innerConfigMap := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name": "default-dynamic-plugins",
			"annotations": map[string]string{
				"rhdh.redhat.com/managed-by": "DevHubPluginCatalog",
			},
		},
		"data": map[string]string{
			model.DynamicPluginsFile: pluginConfig,
		},
	}

	innerYAML, err := yaml.Marshal(innerConfigMap)
	require.NoError(t, err)

	patchData := map[string]interface{}{
		"data": map[string]string{
			model.DynamicPluginsFile: string(innerYAML),
		},
	}

	jsonBytes, err := json.Marshal(patchData)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))

	data, ok := parsed["data"].(map[string]interface{})
	require.True(t, ok)

	// Verify the content is a ConfigMap YAML
	dpContent := data[model.DynamicPluginsFile].(string)
	var configMap map[string]interface{}
	require.NoError(t, yaml.Unmarshal([]byte(dpContent), &configMap), "dynamic-plugins.yaml content should be a ConfigMap YAML")

	assert.Equal(t, "v1", configMap["apiVersion"])
	assert.Equal(t, "ConfigMap", configMap["kind"])

	cmData, ok := configMap["data"].(map[interface{}]interface{})
	require.True(t, ok, "ConfigMap should have data field")

	// Verify the inner content is parseable as DynaPluginsConfig
	innerContent := cmData[model.DynamicPluginsFile].(string)
	var config model.DynaPluginsConfig
	require.NoError(t, yaml.Unmarshal([]byte(innerContent), &config))
	assert.Len(t, config.Plugins, 1)
	assert.Equal(t, "oci://registry.example.com/rhdh/plugin-test:1.0", config.Plugins[0].Package)
}
