package model

import (
	"context"
	"testing"

	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"
	"github.com/redhat-developer/rhdh-operator/pkg/platform"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	"golang.org/x/exp/maps"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

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
		Data: map[string]string{"conf21.yaml": "", "conf22.yaml": ""},
	}

	appConfigTestCm3 = corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-config3",
			Namespace: "ns123",
		},
		Data: map[string]string{"conf31.yaml": "", "conf32.yaml": ""},
	}

	appConfigTestBackstage = bsv1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bs",
			Namespace: "ns123",
		},
		Spec: bsv1.BackstageSpec{
			Application: &bsv1.Application{
				AppConfig: &bsv1.AppConfig{
					MountPath:  "/my/path",
					ConfigMaps: []bsv1.FileObjectRef{},
				},
			},
		},
	}
)

func TestDefaultAppConfig(t *testing.T) {
	__sealights__.StartTestFunc("e5c18eda0dcce016a7", t)
	defer func() { __sealights__.EndTestFunc("e5c18eda0dcce016a7", t) }()

	bs := *appConfigTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("app-config.yaml", "raw-app-config.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Kubernetes, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 1, len(deployment.container().VolumeMounts))
	assert.Contains(t, deployment.container().VolumeMounts[0].MountPath, deployment.defaultMountPath())
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret(AppConfigDefaultName(bs.Name)), deployment.container().VolumeMounts[0].Name)
	assert.Equal(t, 2, len(deployment.container().Args))
	assert.Equal(t, 1, len(deployment.deployment.Spec.Template.Spec.Volumes))

}

func TestSpecifiedAppConfig(t *testing.T) {
	__sealights__.StartTestFunc("298af7a53fb8e32ef9", t)
	defer func() { __sealights__.EndTestFunc("298af7a53fb8e32ef9", t) }()

	bs := *appConfigTestBackstage.DeepCopy()
	bs.Spec.Application.AppConfig.MountPath = "/app/src"
	bs.Spec.Application.AppConfig.ConfigMaps = append(bs.Spec.Application.AppConfig.ConfigMaps,
		bsv1.FileObjectRef{Name: appConfigTestCm.Name})
	bs.Spec.Application.AppConfig.ConfigMaps = append(bs.Spec.Application.AppConfig.ConfigMaps,
		bsv1.FileObjectRef{Name: appConfigTestCm2.Name, MountPath: "/my/appconfig"})
	bs.Spec.Application.AppConfig.ConfigMaps = append(bs.Spec.Application.AppConfig.ConfigMaps,
		bsv1.FileObjectRef{Name: appConfigTestCm3.Name, Key: "conf31.yaml"})

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	testObj.externalConfig.AppConfigKeys = map[string][]string{appConfigTestCm.Name: maps.Keys(appConfigTestCm.Data),
		appConfigTestCm2.Name: maps.Keys(appConfigTestCm2.Data), appConfigTestCm3.Name: maps.Keys(appConfigTestCm3.Data)}

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig,
		platform.Kubernetes, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	// /app/src/conf.yaml
	// /my/appconfig
	// //app/src/conf31.yaml
	assert.Equal(t, 3, len(deployment.container().VolumeMounts))
	assert.Contains(t, deployment.container().VolumeMounts[0].MountPath,
		bs.Spec.Application.AppConfig.MountPath)
	assert.Equal(t, 8, len(deployment.container().Args))
	assert.Equal(t, 3, len(deployment.deployment.Spec.Template.Spec.Volumes))

	assert.Equal(t, "/app/src/conf.yaml", deployment.container().VolumeMounts[0].MountPath)
	assert.Equal(t, "/my/appconfig", deployment.container().VolumeMounts[1].MountPath)
	assert.Equal(t, "/app/src/conf31.yaml", deployment.container().VolumeMounts[2].MountPath)

}

func TestDefaultAndSpecifiedAppConfig(t *testing.T) {
	__sealights__.StartTestFunc("4ff331030d99058960", t)
	defer func() { __sealights__.EndTestFunc("4ff331030d99058960", t) }()

	bs := *appConfigTestBackstage.DeepCopy()
	cms := &bs.Spec.Application.AppConfig.ConfigMaps
	*cms = append(*cms, bsv1.FileObjectRef{Name: appConfigTestCm.Name})

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("app-config.yaml", "raw-app-config.yaml")

	testObj.externalConfig.AppConfigKeys = map[string][]string{appConfigTestCm.Name: maps.Keys(appConfigTestCm.Data)}

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 2, len(deployment.container().VolumeMounts))
	assert.Equal(t, 4, len(deployment.container().Args))
	assert.Equal(t, 2, len(deployment.deployment.Spec.Template.Spec.Volumes))

	assert.Equal(t, deployment.deployment.Spec.Template.Spec.Volumes[0].Name,
		deployment.container().VolumeMounts[0].Name)

}
