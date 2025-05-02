package model

import (
	"context"
	"testing"

	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"
	"github.com/redhat-developer/rhdh-operator/pkg/platform"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	"k8s.io/utils/ptr"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"

	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	__sealights__.StartTestFunc("ed63ee0e52e7946f1b", t)
	defer func() { __sealights__.EndTestFunc("ed63ee0e52e7946f1b", t) }()

	bs := testDynamicPluginsBackstage.DeepCopy()

	testObj := createBackstageTest(*bs).withDefaultConfig(true).
		addToDefaultConfig("dynamic-plugins.yaml", "raw-dynamic-plugins.yaml")

	_, err := InitObjects(context.TODO(), *bs, testObj.externalConfig, platform.Default, testObj.scheme)

	//"failed object validation, reason: failed to find initContainer named install-dynamic-plugins")
	assert.Error(t, err)

}

func TestDynamicPluginsInvalidKeyName(t *testing.T) {
	__sealights__.StartTestFunc("bc6da83df4d586834e", t)
	defer func() { __sealights__.EndTestFunc("bc6da83df4d586834e", t) }()
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
	assert.Contains(t, err.Error(), "expects exactly one Data key named 'dynamic-plugins.yaml'")

}

// Janus specific test
func TestDefaultDynamicPlugins(t *testing.T) {
	__sealights__.StartTestFunc("9663d0e5731a25cf31", t)
	defer func() { __sealights__.EndTestFunc("9663d0e5731a25cf31", t) }()

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

}

func TestDefaultAndSpecifiedDynamicPlugins(t *testing.T) {
	__sealights__.StartTestFunc("06b35adce691538bff", t)
	defer func() { __sealights__.EndTestFunc("06b35adce691538bff", t) }()

	bs := testDynamicPluginsBackstage.DeepCopy()
	bs.Spec.Application.DynamicPluginsConfigMapName = "dplugin"

	testObj := createBackstageTest(*bs).withDefaultConfig(true).
		addToDefaultConfig("dynamic-plugins.yaml", "raw-dynamic-plugins.yaml").
		addToDefaultConfig("deployment.yaml", "janus-deployment.yaml")

	testObj.externalConfig.DynamicPlugins = corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "dplugin"},
		Data:       map[string]string{DynamicPluginsFile: "tt"},
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
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("dplugin"), ic.VolumeMounts[3].Name)
}

func TestDynamicPluginsFailOnArbitraryDepl(t *testing.T) {
	__sealights__.StartTestFunc("08b411fac1c20ceec8", t)
	defer func() { __sealights__.EndTestFunc("08b411fac1c20ceec8", t) }()

	bs := testDynamicPluginsBackstage.DeepCopy()
	//bs.Spec.Application.DynamicPluginsConfigMapName = "dplugin"

	testObj := createBackstageTest(*bs).withDefaultConfig(true).
		addToDefaultConfig("dynamic-plugins.yaml", "raw-dynamic-plugins.yaml")

	_, err := InitObjects(context.TODO(), *bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.Error(t, err)
}

func TestNotConfiguredDPsNotInTheModel(t *testing.T) {
	__sealights__.StartTestFunc("3e956c5e1c938ed3db", t)
	defer func() { __sealights__.EndTestFunc("3e956c5e1c938ed3db", t) }()

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

func initContainer(model *BackstageModel) *corev1.Container {
	__sealights__.TraceFunc("3a6993157e326939fd")
	for _, v := range model.backstageDeployment.deployment.Spec.Template.Spec.InitContainers {
		if v.Name == dynamicPluginInitContainerName {
			return &v
		}
	}
	return nil
}
