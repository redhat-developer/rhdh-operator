package model

import (
	"context"
	"testing"

	"github.com/redhat-developer/rhdh-operator/pkg/platform"

	"k8s.io/utils/ptr"

	"github.com/redhat-developer/rhdh-operator/api"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfigMapEnvFrom(t *testing.T) {

	bs := api.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bs",
			Namespace: "ns123",
		},
		Spec: api.BackstageSpec{
			Database: &api.Database{
				EnableLocalDb: ptr.To(false),
			},
		},
	}

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("configmap-envs.yaml", "raw-cm-envs.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	bscontainer := model.backstageDeployment.container()
	assert.NotNil(t, bscontainer)

	assert.Equal(t, 1, len(bscontainer.EnvFrom))
	assert.Equal(t, 0, len(bscontainer.Env))

}

func TestSpecifiedConfigMapEnvs(t *testing.T) {

	bs := api.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bs",
			Namespace: "ns123",
		},
		Spec: api.BackstageSpec{
			Application: &api.Application{
				ExtraEnvs: &api.ExtraEnvs{
					ConfigMaps: []api.EnvObjectRef{},
				},
			},
			Database: &api.Database{
				EnableLocalDb: ptr.To(false),
			},
		},
	}

	bs.Spec.Application.ExtraEnvs.ConfigMaps = append(bs.Spec.Application.ExtraEnvs.ConfigMaps,
		api.EnvObjectRef{Name: "mapName", Key: "ENV1"})

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	testObj.externalConfig.ExtraEnvConfigMapKeys = map[string]DataObjectKeys{}
	testObj.externalConfig.ExtraEnvConfigMapKeys["mapName"] = NewDataObjectKeys(map[string]string{"mapName": "ENV1"}, nil)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	bscontainer := model.backstageDeployment.container()
	assert.NotNil(t, bscontainer)
	assert.Equal(t, 1, len(bscontainer.Env))

	assert.NotNil(t, bscontainer.Env[0])
	assert.Equal(t, "ENV1", bscontainer.Env[0].ValueFrom.ConfigMapKeyRef.Key)

}

func TestDefaultAndSpecifiedConfigMapEnvFrom(t *testing.T) {

	bs := api.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bs",
			Namespace: "ns123",
		},
		Spec: api.BackstageSpec{
			Application: &api.Application{
				ExtraEnvs: &api.ExtraEnvs{
					ConfigMaps: []api.EnvObjectRef{
						{Name: "mapName", Key: "ENV1"},
					},
				},
			},
			Database: &api.Database{
				EnableLocalDb: ptr.To(false),
			},
		},
	}

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("configmap-envs.yaml", "raw-cm-envs.yaml")

	testObj.externalConfig.ExtraEnvConfigMapKeys = map[string]DataObjectKeys{}
	testObj.externalConfig.ExtraEnvConfigMapKeys["mapName"] = NewDataObjectKeys(map[string]string{"mapName": "ENV1"}, nil)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	bscontainer := model.backstageDeployment.container()
	assert.NotNil(t, bscontainer)

	assert.Equal(t, 1, len(bscontainer.EnvFrom))
	assert.Equal(t, 1, len(bscontainer.Env))

}

func TestSpecifiedCMEnvsWithContainers(t *testing.T) {

	bs := *secretEnvsTestBackstage.DeepCopy()
	bs.Spec.Application = &api.Application{
		ExtraEnvs: &api.ExtraEnvs{
			ConfigMaps: []api.EnvObjectRef{
				{
					Name:       "cmName",
					Key:        "ENV1",
					Containers: []string{"install-dynamic-plugins", "another-container"},
				},
			},
		},
	}

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("deployment.yaml", "multicontainer-deployment.yaml")
	testObj.externalConfig.ExtraEnvSecretKeys = map[string]DataObjectKeys{}
	testObj.externalConfig.ExtraEnvSecretKeys["cmName"] = NewDataObjectKeys(map[string]string{"cmName": "ENV1"}, nil)

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
	bs.Spec.Application.ExtraEnvs.ConfigMaps[0].Containers = []string{"*"}

	testObj = createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("deployment.yaml", "multicontainer-deployment.yaml")
	testObj.externalConfig.ExtraEnvSecretKeys = map[string]DataObjectKeys{}
	testObj.externalConfig.ExtraEnvSecretKeys["cmName"] = NewDataObjectKeys(map[string]string{"cmName": "ENV1"}, nil)

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

func TestCMEnvsWithNonExistedContainerFailed(t *testing.T) {
	bs := *secretEnvsTestBackstage.DeepCopy()
	bs.Spec.Application = &api.Application{
		ExtraEnvs: &api.ExtraEnvs{
			ConfigMaps: []api.EnvObjectRef{
				{
					Name:       "cmName",
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
