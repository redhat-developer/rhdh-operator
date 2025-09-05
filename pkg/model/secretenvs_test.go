package model

import (
	"context"
	"strings"
	"testing"

	"github.com/redhat-developer/rhdh-operator/pkg/platform"

	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"

	"k8s.io/utils/ptr"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

var secretEnvsTestBackstage = bsv1.Backstage{
	ObjectMeta: metav1.ObjectMeta{
		Name: "bs",
	},
	Spec: bsv1.BackstageSpec{
		Database: &bsv1.Database{
			EnableLocalDb: ptr.To(false),
		},
	},
}

func TestDefaultSecretEnvFrom(t *testing.T) {

	bs := *secretEnvsTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig(SecretEnvsObjectKey, "raw-sec-envs.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	bscontainer := model.backstageDeployment.container()
	assert.NotNil(t, bscontainer)

	assert.Equal(t, 1, len(bscontainer.EnvFrom))
	assert.Equal(t, 0, len(bscontainer.Env))

}

func TestDefaultMultiSecretEnv(t *testing.T) {

	bs := *secretEnvsTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("deployment.yaml", "multicontainer-deployment.yaml").
		addToDefaultConfig(SecretEnvsObjectKey, "raw-multi-secret.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	assert.Equal(t, 4, len(model.backstageDeployment.allContainers()))
	assert.Equal(t, 3, len(model.backstageDeployment.container().EnvFrom))
	assert.Equal(t, 2, len(model.backstageDeployment.containerByName("install-dynamic-plugins").EnvFrom))
	assert.Equal(t, 1, len(model.backstageDeployment.containerByName("another-container").EnvFrom))
	mo := model.getRuntimeObjectByType(&SecretEnvs{}).Object().(*multiobject.MultiObject)
	assert.Equal(t, 3, len(mo.Items))
	assert.True(t, strings.HasPrefix(mo.Items[0].GetName(), "backstage-envs-"+bs.Name))
}

func TestSpecifiedSecretEnvs(t *testing.T) {

	bs := *secretEnvsTestBackstage.DeepCopy()
	bs.Spec.Application = &bsv1.Application{
		ExtraEnvs: &bsv1.ExtraEnvs{
			Secrets: []bsv1.EnvObjectRef{},
		},
	}

	bs.Spec.Application.ExtraEnvs.Secrets = append(bs.Spec.Application.ExtraEnvs.Secrets,
		bsv1.EnvObjectRef{Name: "secName", Key: "ENV1"})

	testObj := createBackstageTest(bs).withDefaultConfig(true)
	testObj.externalConfig.ExtraEnvConfigMapKeys = map[string]DataObjectKeys{}
	testObj.externalConfig.ExtraEnvConfigMapKeys["secName"] = NewDataObjectKeys(map[string]string{"secName": "ENV1"}, nil)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	bscontainer := model.backstageDeployment.container()
	assert.NotNil(t, bscontainer)
	assert.Equal(t, 1, len(bscontainer.Env))

	assert.NotNil(t, bscontainer.Env[0])
	assert.Equal(t, "ENV1", bscontainer.Env[0].ValueFrom.SecretKeyRef.Key)

}

func TestSpecifiedSecretEnvsWithContainers(t *testing.T) {

	bs := *secretEnvsTestBackstage.DeepCopy()
	bs.Spec.Application = &bsv1.Application{
		ExtraEnvs: &bsv1.ExtraEnvs{
			Secrets: []bsv1.EnvObjectRef{
				{
					Name:       "secName",
					Key:        "ENV1",
					Containers: []string{"install-dynamic-plugins", "another-container"},
				},
			},
		},
	}

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("deployment.yaml", "multicontainer-deployment.yaml")
	testObj.externalConfig.ExtraEnvSecretKeys = map[string]DataObjectKeys{}
	testObj.externalConfig.ExtraEnvSecretKeys["secName"] = NewDataObjectKeys(map[string]string{"secName": "ENV1"}, nil)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	cont := model.backstageDeployment.containerByName("install-dynamic-plugins")
	assert.NotNil(t, cont)
	assert.Equal(t, 1, len(cont.Env))
	assert.NotNil(t, cont.Env[0])
	assert.Equal(t, "ENV1", cont.Env[0].Name)

	cont = model.backstageDeployment.containerByName("another-container")
	assert.NotNil(t, cont)
	assert.Equal(t, 1, len(cont.Env))
	assert.NotNil(t, cont.Env[0])
	assert.Equal(t, "ENV1", cont.Env[0].Name)

	cont = model.backstageDeployment.containerByName("backstage-backend")
	assert.NotNil(t, cont)
	assert.Equal(t, 0, len(cont.Env))

	// check *
	bs.Spec.Application.ExtraEnvs.Secrets[0].Containers = []string{"*"}

	testObj = createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("deployment.yaml", "multicontainer-deployment.yaml")
	testObj.externalConfig.ExtraEnvSecretKeys = map[string]DataObjectKeys{}
	testObj.externalConfig.ExtraEnvSecretKeys["secName"] = NewDataObjectKeys(map[string]string{"secName": "ENV1"}, nil)

	model, err = InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)
	assert.Equal(t, 4, len(model.backstageDeployment.allContainers()))
	for _, cn := range model.backstageDeployment.allContainers() {
		c := model.backstageDeployment.containerByName(cn)
		assert.Equal(t, 1, len(c.Env))
		assert.NotNil(t, c.Env[0])
		assert.Equal(t, "ENV1", c.Env[0].Name)
	}
}

func TestSecretEnvsWithNonExistedContainerFailed(t *testing.T) {
	bs := *secretEnvsTestBackstage.DeepCopy()
	bs.Spec.Application = &bsv1.Application{
		ExtraEnvs: &bsv1.ExtraEnvs{
			Secrets: []bsv1.EnvObjectRef{
				{
					Name:       "secName",
					Key:        "ENV1",
					Containers: []string{"install-dynamic-plugins", "another-container"},
				},
			},
		},
	}

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	_, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.ErrorContains(t, err, "not found")

}
