package model

import (
	"context"
	"testing"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

var (
	//appConfigTestCm = corev1.ConfigMap{
	//	ObjectMeta: metav1.ObjectMeta{
	//		Name:      "app-config1",
	//		Namespace: "ns123",
	//	},
	//	Data: map[string]string{"conf.yaml": ""},
	//}
	//
	//appConfigTestCm2 = corev1.ConfigMap{
	//	ObjectMeta: metav1.ObjectMeta{
	//		Name:      "app-config2",
	//		Namespace: "ns123",
	//	},
	//	Data: map[string]string{"conf2.yaml": ""},
	//}

	configMapFilesTestBackstage = bsv1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bs",
			Namespace: "ns123",
		},
		Spec: bsv1.BackstageSpec{
			Application: &bsv1.Application{
				ExtraFiles: &bsv1.ExtraFiles{
					MountPath:  "/my/path",
					ConfigMaps: []bsv1.ObjectKeyRef{},
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
	*cmf = append(*cmf, bsv1.ObjectKeyRef{Name: appConfigTestCm.Name})
	*cmf = append(*cmf, bsv1.ObjectKeyRef{Name: appConfigTestCm2.Name})

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, false, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 2, len(deployment.deployment.Spec.Template.Spec.Containers[0].VolumeMounts))
	assert.Equal(t, 0, len(deployment.deployment.Spec.Template.Spec.Containers[0].Args))
	assert.Equal(t, 2, len(deployment.deployment.Spec.Template.Spec.Volumes))

}

func TestDefaultAndSpecifiedConfigMapFiles(t *testing.T) {

	bs := *configMapFilesTestBackstage.DeepCopy()
	cmf := &bs.Spec.Application.ExtraFiles.ConfigMaps
	*cmf = append(*cmf, bsv1.ObjectKeyRef{Name: appConfigTestCm.Name})

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
