package catalog

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/redhat-developer/rhdh-operator/pkg/model"
)

// addPluginsFromYAML is a test helper that parses YAML and adds plugins to the map
func addPluginsFromYAML(t *testing.T, plugins PluginMap, content string) error {
	var config model.DynaPluginsConfig
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		return err
	}
	for _, plugin := range config.Plugins {
		name := plugin.Name()
		if name == "" {
			t.Fatalf("invalid plugin package %q: cannot extract plugin name", plugin.Package)
		}
		if existing, exists := plugins[name]; exists {
			return &duplicatePluginError{name: name, existingPkg: existing.Package, newPkg: plugin.Package}
		}
		plugins[name] = plugin
	}
	return nil
}

type duplicatePluginError struct {
	name        string
	existingPkg string
	newPkg      string
}

func (e *duplicatePluginError) Error() string {
	return "duplicate plugin name " + e.name + ": found in both " + e.existingPkg + " and " + e.newPkg
}

func TestNewProcessor(t *testing.T) {
	p := NewProcessor()
	assert.NotNil(t, p)
}

func TestPluginMap_SingleCatalog(t *testing.T) {
	plugins := make(PluginMap)

	catalog := `plugins:
  - package: "oci://registry.example.com/rhdh/plugin-techdocs:1.0"
    disabled: false
  - package: "oci://registry.example.com/rhdh/plugin-kubernetes:1.0"
    disabled: false
`
	err := addPluginsFromYAML(t, plugins, catalog)
	require.NoError(t, err)
	assert.Len(t, plugins, 2)
}

func TestPluginMap_MultipleCatalogs(t *testing.T) {
	plugins := make(PluginMap)

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
	require.NoError(t, addPluginsFromYAML(t, plugins, catalog1))
	require.NoError(t, addPluginsFromYAML(t, plugins, catalog2))
	assert.Len(t, plugins, 4)
}

func TestPluginMap_DuplicateSameURL_Fails(t *testing.T) {
	plugins := make(PluginMap)

	catalog := `plugins:
  - package: "oci://registry.example.com/rhdh/plugin-techdocs:1.0"
    disabled: false
  - package: "oci://registry.example.com/rhdh/plugin-techdocs:1.0"
    disabled: true
`
	err := addPluginsFromYAML(t, plugins, catalog)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate plugin name")
	assert.Contains(t, err.Error(), "plugin-techdocs")
}

func TestPluginMap_DuplicateDifferentURL_Fails(t *testing.T) {
	plugins := make(PluginMap)

	// Same plugin name but different registries/tags
	catalog := `plugins:
  - package: "oci://registry.example.com/rhdh/plugin-techdocs:1.0"
    disabled: false
  - package: "oci://other-registry.com/path/plugin-techdocs@sha256:abc123"
    disabled: false
`
	err := addPluginsFromYAML(t, plugins, catalog)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate plugin name")
	assert.Contains(t, err.Error(), "plugin-techdocs")
}

func TestPluginMap_DuplicateAcrossCatalogs_Fails(t *testing.T) {
	plugins := make(PluginMap)

	catalog1 := `plugins:
  - package: "oci://registry.example.com/rhdh/plugin-techdocs:1.0"
    disabled: false
`
	catalog2 := `plugins:
  - package: "oci://other-registry.com/rhdh/plugin-techdocs:2.0"
    disabled: false
`
	require.NoError(t, addPluginsFromYAML(t, plugins, catalog1))
	err := addPluginsFromYAML(t, plugins, catalog2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate plugin name")
	assert.Contains(t, err.Error(), "plugin-techdocs")
}

func TestPluginMap_EmptyCatalog(t *testing.T) {
	plugins := make(PluginMap)

	catalog := `plugins: []`

	err := addPluginsFromYAML(t, plugins, catalog)
	require.NoError(t, err)
	assert.Empty(t, plugins)
}

func TestPluginMap_PreservesPluginConfig(t *testing.T) {
	plugins := make(PluginMap)

	catalog := `plugins:
  - package: "oci://registry.example.com/rhdh/plugin-github:1.0"
    disabled: false
    pluginConfig:
      catalog:
        providers:
          github:
            organization: my-org
`
	err := addPluginsFromYAML(t, plugins, catalog)
	require.NoError(t, err)
	assert.Len(t, plugins, 1)
	assert.NotNil(t, plugins["plugin-github"].PluginConfig)
}

func TestBuildPatch(t *testing.T) {
	p := NewProcessor()
	plugins := make(PluginMap)

	plugins["plugin-test"] = model.DynaPlugin{
		Package:  "oci://registry.example.com/rhdh/plugin-test:1.0",
		Disabled: false,
	}

	patchBytes, err := p.BuildPatch(plugins)
	require.NoError(t, err)

	// Verify JSON structure
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(patchBytes, &parsed))

	data, ok := parsed["data"].(map[string]interface{})
	require.True(t, ok)

	// Verify the content is a ConfigMap YAML
	dpContent := data[model.DynamicPluginsFile].(string)
	var configMap map[string]interface{}
	require.NoError(t, yaml.Unmarshal([]byte(dpContent), &configMap))

	assert.Equal(t, "v1", configMap["apiVersion"])
	assert.Equal(t, "ConfigMap", configMap["kind"])
}

func TestBuildPatch_Empty(t *testing.T) {
	p := NewProcessor()
	plugins := make(PluginMap)

	patchBytes, err := p.BuildPatch(plugins)
	require.NoError(t, err)

	// Verify JSON structure
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(patchBytes, &parsed))

	data, ok := parsed["data"].(map[string]interface{})
	require.True(t, ok)

	// Verify the content is a ConfigMap YAML with empty plugins
	dpContent := data[model.DynamicPluginsFile].(string)
	var configMap map[string]interface{}
	require.NoError(t, yaml.Unmarshal([]byte(dpContent), &configMap))

	cmData, ok := configMap["data"].(map[interface{}]interface{})
	require.True(t, ok)

	innerContent := cmData[model.DynamicPluginsFile].(string)
	var config model.DynaPluginsConfig
	require.NoError(t, yaml.Unmarshal([]byte(innerContent), &config))
	assert.Empty(t, config.Plugins)
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
