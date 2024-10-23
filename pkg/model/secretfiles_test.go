package model

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	"k8s.io/utils/ptr"

	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

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
					Secrets:   []bsv1.ObjectKeyRef{},
				},
			},
		},
	}
)

func TestDefaultSecretFiles(t *testing.T) {

	bs := *secretFilesTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("secret-files.yaml", "raw-secret-files.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, false, testObj.scheme)

	assert.NoError(t, err)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 1, len(deployment.deployment.Spec.Template.Spec.Containers[0].VolumeMounts))
	assert.Equal(t, 1, len(deployment.deployment.Spec.Template.Spec.Volumes))

}

func TestSpecifiedSecretFiles(t *testing.T) {

	bs := *secretFilesTestBackstage.DeepCopy()
	sf := &bs.Spec.Application.ExtraFiles.Secrets
	*sf = append(*sf, bsv1.ObjectKeyRef{Name: "secret1", Key: "conf.yaml"})
	*sf = append(*sf, bsv1.ObjectKeyRef{Name: "secret2", Key: "conf.yaml", MountPath: "/custom/path", WithSubPath: ptr.To(false)})
	// https://issues.redhat.com/browse/RHIDP-2246 - mounting secret/CM with dot in the name
	*sf = append(*sf, bsv1.ObjectKeyRef{Name: "secret.dot", Key: "conf3.yaml"})

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, false, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 3, len(deployment.deployment.Spec.Template.Spec.Containers[0].VolumeMounts))
	assert.Equal(t, 0, len(deployment.deployment.Spec.Template.Spec.Containers[0].Args))
	assert.Equal(t, 3, len(deployment.deployment.Spec.Template.Spec.Volumes))

	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("secret1"), deployment.podSpec().Volumes[0].Name)
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("secret2"), deployment.podSpec().Volumes[1].Name)
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("secret.dot"), deployment.podSpec().Volumes[2].Name)

	assert.Equal(t, "conf.yaml", deployment.container().VolumeMounts[0].SubPath)
	assert.Equal(t, "", deployment.container().VolumeMounts[1].SubPath)

	assert.Equal(t, "/custom/path", deployment.container().VolumeMounts[1].MountPath)

	assert.Equal(t, filepath.Join("/my/path", "conf.yaml"), deployment.container().VolumeMounts[0].MountPath)

}

func TestFailedValidation(t *testing.T) {
	bs := *secretFilesTestBackstage.DeepCopy()
	sf := &bs.Spec.Application.ExtraFiles.Secrets
	*sf = append(*sf, bsv1.ObjectKeyRef{Name: "secret1", WithSubPath: ptr.To(false)})

	testObj := createBackstageTest(bs).withDefaultConfig(true)
	_, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, false, testObj.scheme)
	assert.EqualError(t, err, "failed object validation, reason: mounting without subPath to non-individual MountPath is forbidden, Secret name: secret1")

	bs = *secretFilesTestBackstage.DeepCopy()
	sf = &bs.Spec.Application.ExtraFiles.Secrets
	*sf = append(*sf, bsv1.ObjectKeyRef{Name: "secret1", MountPath: "/path", WithSubPath: ptr.To(true)})

	testObj = createBackstageTest(bs).withDefaultConfig(true)
	_, err = InitObjects(context.TODO(), bs, testObj.externalConfig, true, false, testObj.scheme)
	assert.EqualError(t, err, "failed object validation, reason: Key is required if withSubPath is not false to mount extra file from the Secret: secret1")

}

func TestDefaultAndSpecifiedSecretFiles(t *testing.T) {

	bs := *secretFilesTestBackstage.DeepCopy()
	sf := &bs.Spec.Application.ExtraFiles.Secrets
	*sf = append(*sf, bsv1.ObjectKeyRef{Name: "secret1", Key: "conf.yaml"})
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("secret-files.yaml", "raw-secret-files.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, false, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 2, len(deployment.deployment.Spec.Template.Spec.Containers[0].VolumeMounts))
	assert.Equal(t, 0, len(deployment.deployment.Spec.Template.Spec.Containers[0].Args))
	assert.Equal(t, 2, len(deployment.deployment.Spec.Template.Spec.Volumes))
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("secret1"), deployment.podSpec().Volumes[1].Name)

}
