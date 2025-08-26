package model

import (
	"context"
	"testing"

	"github.com/redhat-developer/rhdh-operator/pkg/platform"

	"gopkg.in/yaml.v2"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"k8s.io/utils/ptr"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha4"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

var testDynamicPluginsBackstage = bsv1.Backstage{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "bs",
		Namespace: "ns123",
	},
	Spec: bsv1.BackstageSpec{
		Database: &bsv1.Database{
			EnableLocalDb: ptr.To(false),
		},
		Application: &bsv1.Application{},
	},
}

func TestDynamicPluginsValidationFailed(t *testing.T) {

	bs := testDynamicPluginsBackstage.DeepCopy()

	testObj := createBackstageTest(*bs).withDefaultConfig(true).
		addToDefaultConfig("dynamic-plugins.yaml", "raw-dynamic-plugins.yaml")

	_, err := InitObjects(context.TODO(), *bs, testObj.externalConfig, platform.Default, testObj.scheme)

	//"failed object validation, reason: failed to find initContainer named install-dynamic-plugins")
	assert.Error(t, err)

}

func TestDynamicPluginsInvalidKeyName(t *testing.T) {
	bs := testDynamicPluginsBackstage.DeepCopy()

	bs.Spec.Application.DynamicPluginsConfigMapName = "dplugin"

	testObj := createBackstageTest(*bs).withDefaultConfig(true).
		addToDefaultConfig("dynamic-plugins.yaml", "raw-dynamic-plugins.yaml").
		addToDefaultConfig("deployment.yaml", "janus-deployment.yaml")

	testObj.externalConfig.DynamicPlugins = corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "dplugin"},
		Data:       map[string]string{"WrongKeyName.yml": "tt"},
	}

	_, err := InitObjects(context.TODO(), *bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.Error(t, err)
	//assert.Contains(t, err.Error(), "expects exactly one Data key named 'dynamic-plugins.yaml'")
	assert.Contains(t, err.Error(), "dynamic plugin configMap expects 'dynamic-plugins.yaml' Data key")

}

// Janus specific test
func TestDefaultDynamicPlugins(t *testing.T) {

	bs := testDynamicPluginsBackstage.DeepCopy()

	testObj := createBackstageTest(*bs).withDefaultConfig(true).
		addToDefaultConfig("dynamic-plugins.yaml", "raw-dynamic-plugins.yaml").
		addToDefaultConfig("deployment.yaml", "janus-deployment.yaml")

	model, err := InitObjects(context.TODO(), *bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model.backstageDeployment)
	//dynamic-plugins-root
	//dynamic-plugins-npmrc
	//dynamic-plugins-auth
	//vol-default-dynamic-plugins
	assert.Equal(t, 4, len(model.backstageDeployment.deployment.Spec.Template.Spec.Volumes))

	ic := initContainer(model)
	assert.NotNil(t, ic)
	//dynamic-plugins-root
	//dynamic-plugins-npmrc
	//dynamic-plugins-auth
	//vol-default-dynamic-plugins
	assert.Equal(t, 4, len(ic.VolumeMounts))

	deps, err := model.DynamicPlugins.Dependencies()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(deps))

}

func TestDefaultAndSpecifiedDynamicPlugins(t *testing.T) {

	bs := testDynamicPluginsBackstage.DeepCopy()
	bs.Spec.Application.DynamicPluginsConfigMapName = "dplugin"

	testObj := createBackstageTest(*bs).withDefaultConfig(true).
		addToDefaultConfig("dynamic-plugins.yaml", "raw-dynamic-plugins.yaml").
		addToDefaultConfig("deployment.yaml", "janus-deployment.yaml")

	testObj.externalConfig.DynamicPlugins = corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "dplugin"},
		Data:       map[string]string{DynamicPluginsFile: "dynamic-plugins.yaml: | \n plugins: []"},
	}

	model, err := InitObjects(context.TODO(), *bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	ic := initContainer(model)
	assert.NotNil(t, ic)
	//dynamic-plugins-root
	//dynamic-plugins-npmrc
	//dynamic-plugins-auth
	//vol-dplugin
	assert.Equal(t, 4, len(ic.VolumeMounts))
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret(DynamicPluginsDefaultName(bs.Name)), ic.VolumeMounts[3].Name)

	deps, err := model.DynamicPlugins.Dependencies()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(deps))
}

func TestSpecifiedOnlyDynamicPlugins(t *testing.T) {

	bs := testDynamicPluginsBackstage.DeepCopy()
	bs.Spec.Application.DynamicPluginsConfigMapName = "dplugin"

	testObj := createBackstageTest(*bs).withDefaultConfig(true).
		addToDefaultConfig("deployment.yaml", "janus-deployment.yaml")

	testObj.externalConfig.DynamicPlugins = corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "dplugin"},
		Data:       map[string]string{DynamicPluginsFile: "dynamic-plugins.yaml: | \n plugins: []"},
	}

	model, err := InitObjects(context.TODO(), *bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	ic := initContainer(model)
	assert.NotNil(t, ic)
	//dynamic-plugins-root
	//dynamic-plugins-npmrc
	//dynamic-plugins-auth
	//dplugin
	assert.Equal(t, 4, len(ic.VolumeMounts))
	assert.Equal(t, bs.Spec.Application.DynamicPluginsConfigMapName, ic.VolumeMounts[3].Name)

	deps, err := model.DynamicPlugins.Dependencies()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(deps))
}

func TestDynamicPluginsFailOnArbitraryDepl(t *testing.T) {

	bs := testDynamicPluginsBackstage.DeepCopy()
	//bs.Spec.Application.DynamicPluginsConfigMapName = "dplugin"

	testObj := createBackstageTest(*bs).withDefaultConfig(true).
		addToDefaultConfig("dynamic-plugins.yaml", "raw-dynamic-plugins.yaml")

	_, err := InitObjects(context.TODO(), *bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.Error(t, err)
}

func TestNotConfiguredDPsNotInTheModel(t *testing.T) {

	bs := testDynamicPluginsBackstage.DeepCopy()
	assert.Empty(t, bs.Spec.Application.DynamicPluginsConfigMapName)

	testObj := createBackstageTest(*bs).withDefaultConfig(true)

	m, err := InitObjects(context.TODO(), *bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	for _, obj := range m.RuntimeObjects {
		if _, ok := obj.(*DynamicPlugins); ok {
			assert.Fail(t, "Model contains DynamicPlugins object")
		}
	}
}

func TestWithDynamicPluginsDeps(t *testing.T) {

	bs := testDynamicPluginsBackstage.DeepCopy()
	bs.Spec.Application.DynamicPluginsConfigMapName = "dplugin"

	testObj := createBackstageTest(*bs).withDefaultConfig(true).
		addToDefaultConfig("dynamic-plugins.yaml", "raw-dynamic-plugins.yaml").
		addToDefaultConfig("deployment.yaml", "janus-deployment.yaml")

	yamlData := `"dynamic-plugins.yaml": |
plugins:
  - package: "plugin-a"
    disabled: false
    dependencies:
      - ref: "dependency-1"
      - ref: "dependency-2"
`

	testObj.externalConfig.DynamicPlugins = corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "dplugin"},
		Data:       map[string]string{DynamicPluginsFile: yamlData},
	}

	model, err := InitObjects(context.TODO(), *bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	// dependencies from external config
	//  - ref: "dependency-1"
	//  - ref: "dependency-2"
	deps, err := model.DynamicPlugins.Dependencies()
	assert.NoError(t, err)
	assert.Equal(t, 2, len(deps))

	depends, err := (model.getRuntimeObjectByType(&DynamicPlugins{})).(*DynamicPlugins).Dependencies()
	assert.NoError(t, err)
	assert.Equal(t, 2, len(depends))

}

func initContainer(model *BackstageModel) *corev1.Container {
	for _, v := range model.backstageDeployment.deployment.Spec.Template.Spec.InitContainers {
		if v.Name == dynamicPluginInitContainerName {
			return &v
		}
	}
	return nil
}

func TestUnmarshalDynaPluginsConfig(t *testing.T) {
	yamlData := `
plugins:
  - package: "plugin-a"
    integrity: "sha256-abc123"
    disabled: false
    pluginConfig:
      key1: "value1"
      key2: "value2"
    dependencies:
      - ref: "dependency-1"
      - ref: "dependency-2"
  - package: "plugin-b"
    integrity: "sha256-def456"
    disabled: true
    pluginConfig:
      key3: "value3"
    dependencies: []
`

	var config DynaPluginsConfig
	err := yaml.Unmarshal([]byte(yamlData), &config)
	assert.NoError(t, err)

	// Validate plugins
	assert.Equal(t, 2, len(config.Plugins))

	// Validate first plugin
	pluginA := config.Plugins[0]
	assert.Equal(t, "plugin-a", pluginA.Package)
	assert.Equal(t, "sha256-abc123", pluginA.Integrity)
	assert.False(t, pluginA.Disabled)
	assert.Equal(t, "value1", pluginA.PluginConfig["key1"])
	assert.Equal(t, "value2", pluginA.PluginConfig["key2"])
	assert.Equal(t, 2, len(pluginA.Dependencies))
	assert.Equal(t, "dependency-1", pluginA.Dependencies[0].Ref)
	assert.Equal(t, "dependency-2", pluginA.Dependencies[1].Ref)

	// Validate second plugin
	pluginB := config.Plugins[1]
	assert.Equal(t, "plugin-b", pluginB.Package)
	assert.Equal(t, "sha256-def456", pluginB.Integrity)
	assert.True(t, pluginB.Disabled)
	assert.Equal(t, "value3", pluginB.PluginConfig["key3"])
	assert.Empty(t, pluginB.Dependencies)
}

func TestDynamicPluginsDependencies(t *testing.T) {
	// Case 1: Plugins with dependencies
	yamlDataWithDeps := `
plugins:
  - package: "plugin-a"
    disabled: false
    dependencies:
      - ref: "dependency-1"
      - ref: "dependency-2"
  - package: "plugin-b"
    disabled: false
    dependencies:
      - ref: "dependency-3"
  - package: "plugin-disabled"
    disabled: true
    dependencies:
      - ref: "dependency-4"
`

	dpWithDeps := &DynamicPlugins{
		ConfigMap: &corev1.ConfigMap{
			Data: map[string]string{
				DynamicPluginsFile: yamlDataWithDeps,
			},
		},
	}

	deps, err := dpWithDeps.Dependencies()
	assert.NoError(t, err)
	assert.Equal(t, 3, len(deps))
	assert.Equal(t, "dependency-1", deps[0].Ref)
	assert.Equal(t, "dependency-2", deps[1].Ref)
	assert.Equal(t, "dependency-3", deps[2].Ref)

	// Case 2: Plugins without dependencies
	yamlDataWithoutDeps := `
plugins:
  - package: "plugin-c"
    disabled: false
  - package: "plugin-d"
    disabled: false
`

	dpWithoutDeps := &DynamicPlugins{
		ConfigMap: &corev1.ConfigMap{
			Data: map[string]string{
				DynamicPluginsFile: yamlDataWithoutDeps,
			},
		},
	}

	deps, err = dpWithoutDeps.Dependencies()
	assert.NoError(t, err)
	assert.NotNil(t, deps)
	assert.Equal(t, 0, len(deps)) // Ensure it returns an empty slice, not nil
}

func TestMergeDynamicPlugins(t *testing.T) {
	// Sample model ConfigMap
	modelData := `
plugins:
  - package: "plugin-a"
    integrity: "sha256-abc123"
    disabled: false
    pluginConfig:
      key1: "value1"
    dependencies:
      - ref: "dependency-1"
  - package: "plugin-b"
    integrity: "sha256-def456"
    disabled: true
    pluginConfig:
      key2: "value2"
    dependencies:
      - ref: "dependency-2"
  - package: "plugin-c"
    integrity: "sha256-ghi789"
    pluginConfig:
      key3: "value3"
  - package: "plugin-d"
    disabled: true
    integrity: "sha256-ddd"
    pluginConfig:
      key: "value"
includes:
  - "include-1"
`

	defDynamicPlugins := &DynamicPlugins{
		ConfigMap: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: DynamicPluginsDefaultName("test-backstage"),
			},
			Data: map[string]string{
				DynamicPluginsFile: modelData,
			},
		},
	}

	// Sample spec data
	specData := `
plugins:
  - package: "plugin-a"
    integrity: "sha256-overridden"
    pluginConfig:
      key1: "overridden"
    dependencies:
      - ref: "dependency-3"
  - package: "plugin-d"
  - package: "plugin-e"
includes:
  - "include-2"

`

	// Call the function
	mergedData, err := defDynamicPlugins.mergeWith(specData)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, mergedData)

	// Unmarshal merged data for validation
	var mergedConfig DynaPluginsConfig
	err = yaml.Unmarshal([]byte(mergedData), &mergedConfig)
	assert.NoError(t, err)

	// Validate merged plugins
	assert.Equal(t, 5, len(mergedConfig.Plugins))

	// Validate plugin-a (overridden by specData)
	//pluginA := mergedConfig.Plugins[0]
	//assert.Equal(t, "plugin-a", pluginA.Package)
	pluginA := findPluginByPackage(mergedConfig.Plugins, "plugin-a")
	assert.NotNil(t, pluginA)
	assert.Equal(t, "sha256-overridden", pluginA.Integrity)
	assert.Equal(t, false, pluginA.Disabled)
	assert.Equal(t, "overridden", pluginA.PluginConfig["key1"])
	assert.Equal(t, 1, len(pluginA.Dependencies))
	assert.Equal(t, "dependency-3", pluginA.Dependencies[0].Ref)

	// Validate plugin-b (disabled, from modelDp)
	pluginB := findPluginByPackage(mergedConfig.Plugins, "plugin-b")
	assert.NotNil(t, pluginB)
	assert.Equal(t, true, pluginB.Disabled)

	// Validate plugin-c (from modelDp, as plugin-b is disabled)
	//pluginC := mergedConfig.Plugins[1]
	pluginC := findPluginByPackage(mergedConfig.Plugins, "plugin-c")
	assert.NotNil(t, pluginC)
	//assert.Equal(t, "plugin-c", pluginC.Package)
	assert.Equal(t, "sha256-ghi789", pluginC.Integrity)
	assert.Equal(t, "value3", pluginC.PluginConfig["key3"])

	//pluginD := mergedConfig.Plugins[2]
	pluginD := findPluginByPackage(mergedConfig.Plugins, "plugin-d")
	assert.NotNil(t, pluginD)
	//assert.Equal(t, "plugin-d", pluginD.Package)
	assert.Equal(t, "sha256-ddd", pluginD.Integrity)

	// Validate merged includes
	assert.ElementsMatch(t, []string{"include-1", "include-2"}, mergedConfig.Includes)

	// Marshal the merged configuration into YAML
	marshalledE, err := yaml.Marshal(findPluginByPackage(mergedConfig.Plugins, "plugin-e"))

	assert.NoError(t, err)
	// Validate that the marshalled string omits empty fields
	assert.NotContains(t, string(marshalledE), "integrity", "The string should not contain 'integrity:'")
	// Validate that the marshalled string always includes disabled field
	assert.Contains(t, string(marshalledE), "disabled", "The string should not contain 'disabled:'")
}

func findPluginByPackage(plugins []DynaPlugin, packageName string) *DynaPlugin {
	for _, plugin := range plugins {
		if plugin.Package == packageName {
			return &plugin
		}
	}
	return nil
}
