package model

import (
	"context"
	"testing"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

var (
	configMapFilesTestBackstage = bsv1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bs",
			Namespace: "ns123",
		},
		Spec: bsv1.BackstageSpec{
			Application: &bsv1.Application{
				ExtraFiles: &bsv1.ExtraFiles{
					MountPath:  "/my/path",
					ConfigMaps: []bsv1.FileObjectKeyRef{},
				},
			},
		},
	}
)

func TestDefaultConfigMapFiles(t *testing.T) {

	bs := *configMapFilesTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("configmap-files.yaml", "raw-cm-files.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, false, testObj.scheme)

	assert.NoError(t, err)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 1, len(deployment.deployment.Spec.Template.Spec.Containers[0].VolumeMounts))
	assert.Equal(t, 1, len(deployment.deployment.Spec.Template.Spec.Volumes))

}

func TestSpecifiedConfigMapFiles(t *testing.T) {

	bs := *configMapFilesTestBackstage.DeepCopy()
	cmf := &bs.Spec.Application.ExtraFiles.ConfigMaps
	*cmf = append(*cmf, bsv1.FileObjectKeyRef{Name: "cm1"})
	*cmf = append(*cmf, bsv1.FileObjectKeyRef{Name: "cm2", MountPath: "/custom/path"})
	*cmf = append(*cmf, bsv1.FileObjectKeyRef{Name: "cm3", MountPath: "rel"})

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	testObj.externalConfig.ExtraFileConfigMaps = map[string]corev1.ConfigMap{}
	testObj.externalConfig.ExtraFileConfigMaps["cm1"] = corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm1"}, Data: map[string]string{"conf1.yaml": "data"}}
	testObj.externalConfig.ExtraFileConfigMaps["cm2"] = corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm2"}, Data: map[string]string{"conf2.yaml": "data"}}
	testObj.externalConfig.ExtraFileConfigMaps["cm3"] = corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm3"}, Data: map[string]string{"conf3.yaml": "data"}}

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, false, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 3, len(deployment.deployment.Spec.Template.Spec.Containers[0].VolumeMounts))
	assert.Equal(t, 0, len(deployment.deployment.Spec.Template.Spec.Containers[0].Args))
	assert.Equal(t, 3, len(deployment.deployment.Spec.Template.Spec.Volumes))

	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("cm1"), deployment.container().VolumeMounts[0].Name)
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("cm2"), deployment.container().VolumeMounts[1].Name)

	assert.Equal(t, "/my/path/conf1.yaml", deployment.container().VolumeMounts[0].MountPath)
	assert.Equal(t, "/custom/path", deployment.container().VolumeMounts[1].MountPath)
	assert.Equal(t, "/my/path/rel", deployment.container().VolumeMounts[2].MountPath)

	assert.Equal(t, "conf1.yaml", deployment.container().VolumeMounts[0].SubPath)
	assert.Equal(t, "", deployment.container().VolumeMounts[1].SubPath)
	assert.Equal(t, "", deployment.container().VolumeMounts[2].SubPath)

}

func TestDefaultAndSpecifiedConfigMapFiles(t *testing.T) {

	bs := *configMapFilesTestBackstage.DeepCopy()
	cmf := &bs.Spec.Application.ExtraFiles.ConfigMaps
	*cmf = append(*cmf, bsv1.FileObjectKeyRef{Name: appConfigTestCm.Name})

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("configmap-files.yaml", "raw-cm-files.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, false, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 2, len(deployment.deployment.Spec.Template.Spec.Containers[0].VolumeMounts))
	assert.Equal(t, 0, len(deployment.deployment.Spec.Template.Spec.Containers[0].Args))
	assert.Equal(t, 2, len(deployment.deployment.Spec.Template.Spec.Volumes))

}
