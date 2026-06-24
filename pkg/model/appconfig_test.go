package model

import (
	"context"
	"testing"

	"github.com/redhat-developer/rhdh-operator/pkg/platform"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	"golang.org/x/exp/maps"

	"github.com/redhat-developer/rhdh-operator/api"

	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	appConfigTestCm = corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-config1",
			Namespace: "ns123",
		},
		Data: map[string]string{"conf.yaml": "conf.yaml data"},
	}

	appConfigTestCm2 = corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-config2",
			Namespace: "ns123",
		},
		Data: map[string]string{"conf21.yaml": ""},
	}

	appConfigTestCm3 = corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-config3",
			Namespace: "ns123",
		},
		Data: map[string]string{"conf31.yaml": ""},
	}

	appConfigTestBackstage = api.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bsName",
			Namespace: "ns123",
		},
		Spec: api.BackstageSpec{
			Application: &api.Application{
				AppConfig: &api.AppConfig{
					MountPath:  "/my/path",
					ConfigMaps: []api.FileObjectRef{},
				},
			},
			Database: &api.Database{},
		},
	}
)

func TestDefaultAppConfig(t *testing.T) {

	bs := *appConfigTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("app-config.yaml", "raw-app-config.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Kubernetes, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.GetRuntimeObjects()) > 0)

	deployment := model.GetRuntimeObject(DeploymentKey).(*BackstageDeployment)
	assert.NotNil(t, deployment)

	assert.Equal(t, 1, len(deployment.container().VolumeMounts))
	assert.Contains(t, deployment.container().VolumeMounts[0].MountPath, deployment.defaultMountPath())
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret(DefaultMultiObjectName("appconfig", bs.Name, "my-backstage-config-cm1")), deployment.container().VolumeMounts[0].Name)
	assert.Equal(t, 2, len(deployment.container().Args))
	assert.Equal(t, 1, len(deployment.podSpec().Volumes))

}

func TestSpecifiedAppConfig(t *testing.T) {

	bs := *appConfigTestBackstage.DeepCopy()
	bs.Spec.Application.AppConfig.MountPath = "/app/src"
	bs.Spec.Application.AppConfig.ConfigMaps = append(bs.Spec.Application.AppConfig.ConfigMaps,
		api.FileObjectRef{Name: appConfigTestCm.Name})
	bs.Spec.Application.AppConfig.ConfigMaps = append(bs.Spec.Application.AppConfig.ConfigMaps,
		api.FileObjectRef{Name: appConfigTestCm2.Name, MountPath: "/my/appconfig"})
	bs.Spec.Application.AppConfig.ConfigMaps = append(bs.Spec.Application.AppConfig.ConfigMaps,
		api.FileObjectRef{Name: appConfigTestCm3.Name, Key: "conf31.yaml"})

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	testObj.externalConfig.AppConfigKeys = map[string][]string{appConfigTestCm.Name: maps.Keys(appConfigTestCm.Data),
		appConfigTestCm2.Name: maps.Keys(appConfigTestCm2.Data), appConfigTestCm3.Name: maps.Keys(appConfigTestCm3.Data)}

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig,
		platform.Kubernetes, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.GetRuntimeObjects()) > 0)

	deployment := model.GetRuntimeObject(DeploymentKey).(*BackstageDeployment)
	assert.NotNil(t, deployment)

	// /app/src/conf.yaml
	// /my/appconfig
	// //app/src/conf31.yaml
	assert.Equal(t, 3, len(deployment.container().VolumeMounts))
	assert.Contains(t, deployment.container().VolumeMounts[0].MountPath,
		bs.Spec.Application.AppConfig.MountPath)
	assert.Equal(t, 6, len(deployment.container().Args))
	assert.Equal(t, 3, len(deployment.podSpec().Volumes))

	assert.Equal(t, "/app/src/conf.yaml", deployment.container().VolumeMounts[0].MountPath)
	assert.Equal(t, "/my/appconfig", deployment.container().VolumeMounts[1].MountPath)
	assert.Equal(t, "/app/src/conf31.yaml", deployment.container().VolumeMounts[2].MountPath)

}

func TestDefaultAndSpecifiedAppConfig(t *testing.T) {

	bs := *appConfigTestBackstage.DeepCopy()
	cms := &bs.Spec.Application.AppConfig.ConfigMaps
	*cms = append(*cms, api.FileObjectRef{Name: appConfigTestCm.Name})

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("app-config.yaml", "raw-app-config.yaml")

	testObj.externalConfig.AppConfigKeys = map[string][]string{appConfigTestCm.Name: maps.Keys(appConfigTestCm.Data)}

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.GetRuntimeObjects()) > 0)

	deployment := model.GetRuntimeObject(DeploymentKey).(*BackstageDeployment)
	assert.NotNil(t, deployment)

	assert.Equal(t, 2, len(deployment.container().VolumeMounts))
	assert.Equal(t, 4, len(deployment.container().Args))
	assert.Equal(t, 2, len(deployment.podSpec().Volumes))

	assert.Equal(t, deployment.podSpec().Volumes[0].Name,
		deployment.container().VolumeMounts[0].Name)

}

// TestMultiEntryAppConfigNotAllowed verifies that ConfigMaps with multiple entries
// are rejected to ensure predictable order in the app-config chain.
func TestMultiEntryAppConfigNotAllowed(t *testing.T) {
	bs := *appConfigTestBackstage.DeepCopy()

	// Reference a ConfigMap with multiple entries
	multiEntryCmName := "multi-entry-config"
	bs.Spec.Application.AppConfig.ConfigMaps = []api.FileObjectRef{
		{Name: multiEntryCmName},
	}

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	// Simulate a ConfigMap with multiple data entries
	testObj.externalConfig.AppConfigKeys = map[string][]string{
		multiEntryCmName: {"config1.yaml", "config2.yaml"}, // Multiple entries - should fail
	}

	_, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "multiple entries")
	assert.Contains(t, err.Error(), multiEntryCmName)
}
