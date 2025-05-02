package model

import (
	"context"
	"testing"

	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/platform"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	"k8s.io/utils/ptr"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	__sealights__.StartTestFunc("39d5aebf7144122a90", t)
	defer func() { __sealights__.EndTestFunc("39d5aebf7144122a90", t) }()

	bs := *secretFilesTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig(SecretFilesObjectKey, "raw-secret-files.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 1, len(deployment.container().VolumeMounts))
	assert.Equal(t, 1, len(deployment.deployment.Spec.Template.Spec.Volumes))

}

func TestDefaultMultiSecretFiles(t *testing.T) {
	__sealights__.StartTestFunc("6c660541625d899207", t)
	defer func() { __sealights__.EndTestFunc("6c660541625d899207", t) }()

	bs := bsv1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bs",
		},
		Spec: bsv1.BackstageSpec{
			Database: &bsv1.Database{
				EnableLocalDb: ptr.To(false),
			},
		},
	}

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("deployment.yaml", "multicontainer-deployment.yaml").
		addToDefaultConfig(SecretFilesObjectKey, "raw-multi-secret.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	mo := model.getRuntimeObjectByType(&SecretFiles{}).Object().(*multiobject.MultiObject)
	assert.Equal(t, 3, len(mo.Items))
	// data1,data2,data3+data4,data5
	assert.Equal(t, 4, len(model.backstageDeployment.container().VolumeMounts))
	// data1,data2,data5
	assert.Equal(t, 3, len(model.backstageDeployment.containerByName("install-dynamic-plugins").VolumeMounts))
	// data5
	assert.Equal(t, 1, len(model.backstageDeployment.containerByName("another-container").VolumeMounts))
	assert.Equal(t, 3, len(model.backstageDeployment.deployment.Spec.Template.Spec.Volumes))
}

func TestSpecifiedSecretFiles(t *testing.T) {
	__sealights__.StartTestFunc("8aed78e0287483d524", t)
	defer func() { __sealights__.EndTestFunc("8aed78e0287483d524", t) }()

	bs := *secretFilesTestBackstage.DeepCopy()
	sf := &bs.Spec.Application.ExtraFiles.Secrets
	// 0 - expected subPath="conf.yaml", expected defaultMountPath=/
	*sf = append(*sf, bsv1.FileObjectRef{Name: "secret1", Key: "conf.yaml"})
	*sf = append(*sf, bsv1.FileObjectRef{Name: "secret2", MountPath: "/custom/path"})
	// https://issues.redhat.com/browse/RHIDP-2246 - mounting secret/CM with dot in the name
	*sf = append(*sf, bsv1.FileObjectRef{Name: "secret.dot", Key: "conf3.yaml"})

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	testObj.externalConfig.ExtraFileSecretKeys = map[string]DataObjectKeys{}
	testObj.externalConfig.ExtraFileSecretKeys["secret1"] = NewDataObjectKeys(nil, nil)
	testObj.externalConfig.ExtraFileSecretKeys["secret2"] = NewDataObjectKeys(map[string]string{"conf.yaml": "data"}, nil)
	testObj.externalConfig.ExtraFileSecretKeys["secret.dot"] = NewDataObjectKeys(nil, map[string][]byte{"conf3.yaml": []byte("data")})

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

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
	__sealights__.StartTestFunc("9210ff046a48235ed4", t)
	defer func() { __sealights__.EndTestFunc("9210ff046a48235ed4", t) }()
	bs := *secretFilesTestBackstage.DeepCopy()
	sf := &bs.Spec.Application.ExtraFiles.Secrets
	*sf = append(*sf, bsv1.FileObjectRef{Name: "secret1"})

	testObj := createBackstageTest(bs).withDefaultConfig(true)
	_, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.EqualError(t, err, "key is required if defaultMountPath is not specified for secret secret1")

}

func TestDefaultAndSpecifiedSecretFiles(t *testing.T) {
	__sealights__.StartTestFunc("49dffeebd06d30a775", t)
	defer func() { __sealights__.EndTestFunc("49dffeebd06d30a775", t) }()

	bs := *secretFilesTestBackstage.DeepCopy()
	sf := &bs.Spec.Application.ExtraFiles.Secrets
	*sf = append(*sf, bsv1.FileObjectRef{Name: "secret1", Key: "conf.yaml"})
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("secret-files.yaml", "raw-secret-files.yaml")

	testObj.externalConfig.ExtraFileSecretKeys = map[string]DataObjectKeys{"secret1": NewDataObjectKeys(map[string]string{"conf.yaml": ""}, nil)}

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 2, len(deployment.container().VolumeMounts))
	assert.Equal(t, 0, len(deployment.container().Args))
	assert.Equal(t, 2, len(deployment.deployment.Spec.Template.Spec.Volumes))
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("secret1"), deployment.podSpec().Volumes[1].Name)

}

func TestSpecifiedSecretFilesWithDataAndKey(t *testing.T) {
	__sealights__.StartTestFunc("b68472b477319560f9", t)
	defer func() { __sealights__.EndTestFunc("b68472b477319560f9", t) }()

	bs := *secretFilesTestBackstage.DeepCopy()
	sf := &bs.Spec.Application.ExtraFiles.Secrets
	*sf = append(*sf, bsv1.FileObjectRef{Name: "secret1", Key: "conf.yaml"})
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("secret-files.yaml", "raw-secret-files.yaml")

	testObj.externalConfig.ExtraFileSecretKeys = map[string]DataObjectKeys{"secret1": NewDataObjectKeys(nil, map[string][]byte{"conf.yaml": []byte("")})}

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 2, len(deployment.container().VolumeMounts))
	assert.Equal(t, 0, len(deployment.container().Args))
	assert.Equal(t, 2, len(deployment.deployment.Spec.Template.Spec.Volumes))
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("secret1"), deployment.podSpec().Volumes[1].Name)

}
