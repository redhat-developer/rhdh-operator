package model

import (
	"context"
	"testing"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/platform"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var testFlavoursBackstage = api.Backstage{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "bs",
		Namespace: "ns123",
	},
	Spec: api.BackstageSpec{
		Application: &api.Application{
			AppConfig: &api.AppConfig{
				MountPath:  "/my/path",
				ConfigMaps: []api.FileObjectRef{},
			},
		},
	},
}

func TestFlavoursWithDefaultsEnabled(t *testing.T) {
	bs := testFlavoursBackstage.DeepCopy()
	// No flavours specified - should use enabledByDefault

	testObj := createBackstageTest(*bs).withConfigPath("./testdata/testflavours").withLocalDb(false)

	model, err := InitObjects(context.TODO(), testObj.backstage, testObj.externalConfig, platform.Kubernetes, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	// Verify app-config: should have base + flavor1 (enabledByDefault: true)
	assert.NotNil(t, findConfigMapByName(model.appConfig.ConfigMaps.Items, "backstage-appconfig-bs"), "base app-config should exist")
	assert.NotNil(t, findConfigMapBySource(model.appConfig.ConfigMaps.Items, "flavour-flavor1"), "flavor1 flavour app-config should exist")

	// Verify configmap-files: should have base + flavor1
	cmFiles := model.getRuntimeObjectByType(&ConfigMapFiles{}).(*ConfigMapFiles)
	assert.NotNil(t, cmFiles)
	assert.NotNil(t, findConfigMapByName(cmFiles.ConfigMaps.Items, "backstage-files-bs"), "base configmap-files should exist")
	assert.NotNil(t, findConfigMapBySource(cmFiles.ConfigMaps.Items, "flavour-flavor1"), "flavor1 flavour configmap-files should exist")

	// Verify configmap-envs: should have base + flavor1
	cmEnvs := model.getRuntimeObjectByType(&ConfigMapEnvs{}).(*ConfigMapEnvs)
	assert.NotNil(t, cmEnvs)
	assert.NotNil(t, findConfigMapByName(cmEnvs.ConfigMaps.Items, "backstage-envs-bs"), "base configmap-envs should exist")
	assert.NotNil(t, findConfigMapBySource(cmEnvs.ConfigMaps.Items, "flavour-flavor1"), "flavor1 flavour configmap-envs should exist")

	// Verify deployment: should have flavor1 label and env var merged
	assert.NotNil(t, model.backstageDeployment)
	assert.Equal(t, "flavor1", model.backstageDeployment.deployable.GetObject().GetLabels()["flavor"], "deployment should have flavor1 label")
	container := model.backstageDeployment.container()
	assert.NotNil(t, findEnvVar(container.Env, "FLAVOR1_ENABLED"), "deployment should have FLAVOR1_ENABLED env var")

	// Verify dynamic-plugins: should have base + flavor1 plugins
	var dpConfig DynaPluginsConfig
	err = yaml.Unmarshal([]byte(model.DynamicPlugins.ConfigMap.Data[DynamicPluginsFile]), &dpConfig)
	assert.NoError(t, err)
	assert.NotNil(t, findPluginByPackage(dpConfig.Plugins, "plugin-base"))
	assert.NotNil(t, findPluginByPackage(dpConfig.Plugins, "plugin-flavor1"))
}

func TestFlavoursWithExplicitEnabled(t *testing.T) {
	bs := testFlavoursBackstage.DeepCopy()
	bs.Spec.Flavours = []api.Flavour{
		{Name: "flavor2", Enabled: true},
	}

	testObj := createBackstageTest(*bs).withConfigPath("./testdata/testflavours").withLocalDb(false)

	model, err := InitObjects(context.TODO(), testObj.backstage, testObj.externalConfig, platform.Kubernetes, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	// Verify app-config: should have base + flavor2 + flavor1 (default not disabled)
	assert.NotNil(t, findConfigMapByName(model.appConfig.ConfigMaps.Items, "backstage-appconfig-bs"), "base app-config should exist")
	assert.NotNil(t, findConfigMapBySource(model.appConfig.ConfigMaps.Items, "flavour-flavor2"), "flavor2 flavour app-config should exist")
	assert.NotNil(t, findConfigMapBySource(model.appConfig.ConfigMaps.Items, "flavour-flavor1"), "flavor1 flavour app-config should exist")

	// Verify configmap-files: should have base + flavor2 + flavor1
	cmFiles := model.getRuntimeObjectByType(&ConfigMapFiles{}).(*ConfigMapFiles)
	assert.NotNil(t, cmFiles)
	assert.NotNil(t, findConfigMapByName(cmFiles.ConfigMaps.Items, "backstage-files-bs"), "base configmap-files should exist")
	assert.NotNil(t, findConfigMapBySource(cmFiles.ConfigMaps.Items, "flavour-flavor2"), "flavor2 flavour configmap-files should exist")
	assert.NotNil(t, findConfigMapBySource(cmFiles.ConfigMaps.Items, "flavour-flavor1"), "flavor1 flavour configmap-files should exist")

	// Verify configmap-envs: should have base + flavor2 + flavor1
	cmEnvs := model.getRuntimeObjectByType(&ConfigMapEnvs{}).(*ConfigMapEnvs)
	assert.NotNil(t, cmEnvs)
	assert.NotNil(t, findConfigMapByName(cmEnvs.ConfigMaps.Items, "backstage-envs-bs"), "base configmap-envs should exist")
	assert.NotNil(t, findConfigMapBySource(cmEnvs.ConfigMaps.Items, "flavour-flavor2"), "flavor2 flavour configmap-envs should exist")
	assert.NotNil(t, findConfigMapBySource(cmEnvs.ConfigMaps.Items, "flavour-flavor1"), "flavor1 flavour configmap-envs should exist")

	// Verify deployment: should have flavor2 label (later flavours override earlier ones)
	assert.NotNil(t, model.backstageDeployment)
	assert.Equal(t, "flavor2", model.backstageDeployment.deployable.GetObject().GetLabels()["flavor"], "deployment should have flavor2 label (overrides flavor1)")
	container := model.backstageDeployment.container()
	assert.NotNil(t, findEnvVar(container.Env, "FLAVOR1_ENABLED"), "deployment should have FLAVOR1_ENABLED env var")
	assert.NotNil(t, findEnvVar(container.Env, "FLAVOR2_ENABLED"), "deployment should have FLAVOR2_ENABLED env var")

	// Verify dynamic-plugins: should have all three
	var dpConfig DynaPluginsConfig
	err = yaml.Unmarshal([]byte(model.DynamicPlugins.ConfigMap.Data[DynamicPluginsFile]), &dpConfig)
	assert.NoError(t, err)
	assert.NotNil(t, findPluginByPackage(dpConfig.Plugins, "plugin-base"))
	assert.NotNil(t, findPluginByPackage(dpConfig.Plugins, "plugin-flavor2"))
	assert.NotNil(t, findPluginByPackage(dpConfig.Plugins, "plugin-flavor1"))
}

func TestFlavoursWithDefaultDisabled(t *testing.T) {
	bs := testFlavoursBackstage.DeepCopy()
	bs.Spec.Flavours = []api.Flavour{
		{Name: "flavor1", Enabled: false}, // Disable default flavour
	}

	testObj := createBackstageTest(*bs).withConfigPath("./testdata/testflavours").withLocalDb(false)

	model, err := InitObjects(context.TODO(), testObj.backstage, testObj.externalConfig, platform.Kubernetes, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	// Verify app-config: should have only base (flavor1 disabled)
	assert.NotNil(t, findConfigMapByName(model.appConfig.ConfigMaps.Items, "backstage-appconfig-bs"), "base app-config should exist")
	assert.Nil(t, findConfigMapBySource(model.appConfig.ConfigMaps.Items, "flavour-flavor1"), "flavor1 flavour app-config should NOT exist")

	// Verify configmap-files: should have only base (flavor1 disabled)
	cmFiles := model.getRuntimeObjectByType(&ConfigMapFiles{}).(*ConfigMapFiles)
	assert.NotNil(t, cmFiles)
	assert.NotNil(t, findConfigMapByName(cmFiles.ConfigMaps.Items, "backstage-files-bs"), "base configmap-files should exist")
	assert.Nil(t, findConfigMapBySource(cmFiles.ConfigMaps.Items, "flavour-flavor1"), "flavor1 flavour configmap-files should NOT exist")

	// Verify configmap-envs: should have only base (flavor1 disabled)
	cmEnvs := model.getRuntimeObjectByType(&ConfigMapEnvs{}).(*ConfigMapEnvs)
	assert.NotNil(t, cmEnvs)
	assert.NotNil(t, findConfigMapByName(cmEnvs.ConfigMaps.Items, "backstage-envs-bs"), "base configmap-envs should exist")
	assert.Nil(t, findConfigMapBySource(cmEnvs.ConfigMaps.Items, "flavour-flavor1"), "flavor1 flavour configmap-envs should NOT exist")

	// Verify deployment: should NOT have flavor1 label or env var
	assert.NotNil(t, model.backstageDeployment)
	assert.Empty(t, model.backstageDeployment.deployable.GetObject().GetLabels()["flavor"], "deployment should NOT have flavor label")
	container := model.backstageDeployment.container()
	assert.Nil(t, findEnvVar(container.Env, "FLAVOR1_ENABLED"), "deployment should NOT have FLAVOR1_ENABLED env var")

	// Verify dynamic-plugins: should have only base plugin
	var dpConfig DynaPluginsConfig
	err = yaml.Unmarshal([]byte(model.DynamicPlugins.ConfigMap.Data[DynamicPluginsFile]), &dpConfig)
	assert.NoError(t, err)
	assert.NotNil(t, findPluginByPackage(dpConfig.Plugins, "plugin-base"))
	assert.Nil(t, findPluginByPackage(dpConfig.Plugins, "plugin-flavor1"))
}

func TestFlavoursOnlyNoBase(t *testing.T) {
	bs := testFlavoursBackstage.DeepCopy()
	bs.Spec.Flavours = []api.Flavour{
		{Name: "flavor3", Enabled: true}, // Enable flavour when no base config exists
	}

	testObj := createBackstageTest(*bs).withConfigPath("./testdata/testflavours-nobase").withLocalDb(false)

	model, err := InitObjects(context.TODO(), testObj.backstage, testObj.externalConfig, platform.Kubernetes, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	// Verify app-config: should have only flavor3 flavour (no base)
	assert.Nil(t, findConfigMapByName(model.appConfig.ConfigMaps.Items, "backstage-appconfig-bs"), "base app-config should NOT exist")
	assert.NotNil(t, findConfigMapBySource(model.appConfig.ConfigMaps.Items, "flavour-flavor3"), "flavor3 flavour app-config should exist")

	// Verify configmap-files: should have only flavor3 flavour (no base)
	cmFiles := model.getRuntimeObjectByType(&ConfigMapFiles{}).(*ConfigMapFiles)
	assert.NotNil(t, cmFiles)
	assert.Nil(t, findConfigMapByName(cmFiles.ConfigMaps.Items, "backstage-files-bs"), "base configmap-files should NOT exist")
	assert.NotNil(t, findConfigMapBySource(cmFiles.ConfigMaps.Items, "flavour-flavor3"), "flavor3 flavour configmap-files should exist")

	// Verify configmap-envs: should have only flavor3 flavour (no base)
	cmEnvs := model.getRuntimeObjectByType(&ConfigMapEnvs{}).(*ConfigMapEnvs)
	assert.NotNil(t, cmEnvs)
	assert.Nil(t, findConfigMapByName(cmEnvs.ConfigMaps.Items, "backstage-envs-bs"), "base configmap-envs should NOT exist")
	assert.NotNil(t, findConfigMapBySource(cmEnvs.ConfigMaps.Items, "flavour-flavor3"), "flavor3 flavour configmap-envs should exist")

	// Verify dynamic-plugins: should have only flavor3 plugin (no base)
	var dpConfig DynaPluginsConfig
	err = yaml.Unmarshal([]byte(model.DynamicPlugins.ConfigMap.Data[DynamicPluginsFile]), &dpConfig)
	assert.NoError(t, err)
	assert.Nil(t, findPluginByPackage(dpConfig.Plugins, "plugin-base"), "base plugin should NOT exist")
	assert.NotNil(t, findPluginByPackage(dpConfig.Plugins, "plugin-flavor3"), "flavor3 plugin should exist")
}
