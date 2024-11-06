package model

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

var (
	secretFilesTestBackstage = bsv1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bs",
			Namespace: "ns123",
		},
		Spec: bsv1.BackstageSpec{
			Application: &bsv1.Application{
				ExtraFiles: &bsv1.ExtraFiles{
					MountPath: "/my/path",
					Secrets:   []bsv1.FileObjectRef{},
				},
			},
		},
	}
)

func TestDefaultSecretFiles(t *testing.T) {

	bs := *secretFilesTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("secret-files.yaml", "raw-secret-files.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, false, testObj.scheme)

	assert.NoError(t, err)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 1, len(deployment.container().VolumeMounts))
	assert.Equal(t, 1, len(deployment.deployment.Spec.Template.Spec.Volumes))

}

func TestSpecifiedSecretFiles(t *testing.T) {

	bs := *secretFilesTestBackstage.DeepCopy()
	sf := &bs.Spec.Application.ExtraFiles.Secrets
	// 0 - expected subPath="conf.yaml", expected defaultMountPath=/
	*sf = append(*sf, bsv1.FileObjectRef{Name: "secret1", Key: "conf.yaml"})
	*sf = append(*sf, bsv1.FileObjectRef{Name: "secret2", MountPath: "/custom/path"})
	// https://issues.redhat.com/browse/RHIDP-2246 - mounting secret/CM with dot in the name
	*sf = append(*sf, bsv1.FileObjectRef{Name: "secret.dot", Key: "conf3.yaml"})

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	testObj.externalConfig.ExtraFileSecrets = map[string]corev1.Secret{}
	testObj.externalConfig.ExtraFileSecrets["secret1"] = corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret1"} /*, StringData: map[string]string{"conf.yaml": "data"}*/} // no data possible if no permissions to read the Secret
	testObj.externalConfig.ExtraFileSecrets["secret2"] = corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret2"}, StringData: map[string]string{"conf.yaml": "data"}}
	testObj.externalConfig.ExtraFileSecrets["secret.dot"] = corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret.dot"}, StringData: map[string]string{"conf3.yaml": "data"}}

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, false, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 3, len(deployment.container().VolumeMounts))
	assert.Equal(t, 0, len(deployment.container().Args))
	assert.Equal(t, 3, len(deployment.deployment.Spec.Template.Spec.Volumes))

	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("secret1"), deployment.podSpec().Volumes[0].Name)
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("secret2"), deployment.podSpec().Volumes[1].Name)
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("secret.dot"), deployment.podSpec().Volumes[2].Name)

	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("secret1"), deployment.container().VolumeMounts[0].Name)
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("secret2"), deployment.container().VolumeMounts[1].Name)
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("secret.dot"), deployment.container().VolumeMounts[2].Name)

	assert.Equal(t, "/my/path/conf.yaml", deployment.container().VolumeMounts[0].MountPath)
	assert.Equal(t, "/custom/path", deployment.container().VolumeMounts[1].MountPath)

	assert.Equal(t, "conf.yaml", deployment.container().VolumeMounts[0].SubPath)
	assert.Equal(t, "", deployment.container().VolumeMounts[1].SubPath)

}

func TestFailedValidation(t *testing.T) {
	bs := *secretFilesTestBackstage.DeepCopy()
	sf := &bs.Spec.Application.ExtraFiles.Secrets
	*sf = append(*sf, bsv1.FileObjectRef{Name: "secret1"})

	testObj := createBackstageTest(bs).withDefaultConfig(true)
	_, err := InitObjects(context.TODO(), bs, testObj.externalConfig, false, testObj.scheme)
	assert.EqualError(t, err, "key is required if defaultMountPath is not specified for secret secret1")

}

func TestDefaultAndSpecifiedSecretFiles(t *testing.T) {

	bs := *secretFilesTestBackstage.DeepCopy()
	sf := &bs.Spec.Application.ExtraFiles.Secrets
	*sf = append(*sf, bsv1.FileObjectRef{Name: "secret1", Key: "conf.yaml"})
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("secret-files.yaml", "raw-secret-files.yaml")

	testObj.externalConfig.ExtraFileSecrets = map[string]corev1.Secret{"secret1": corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret1"}, StringData: map[string]string{"conf.yaml": ""}}}

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, false, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 2, len(deployment.container().VolumeMounts))
	assert.Equal(t, 0, len(deployment.container().Args))
	assert.Equal(t, 2, len(deployment.deployment.Spec.Template.Spec.Volumes))
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("secret1"), deployment.podSpec().Volumes[1].Name)

}
