package model

import (
	"context"
	"testing"

	"k8s.io/utils/ptr"

	corev1 "k8s.io/api/core/v1"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfigMapEnvFrom(t *testing.T) {

	bs := bsv1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bs",
			Namespace: "ns123",
		},
		Spec: bsv1.BackstageSpec{
			Database: &bsv1.Database{
				EnableLocalDb: ptr.To(false),
			},
		},
	}

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("configmap-envs.yaml", "raw-cm-envs.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, false, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	bscontainer := model.backstageDeployment.container()
	assert.NotNil(t, bscontainer)

	assert.Equal(t, 1, len(bscontainer.EnvFrom))
	assert.Equal(t, 0, len(bscontainer.Env))

}

func TestSpecifiedConfigMapEnvs(t *testing.T) {

	bs := bsv1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bs",
			Namespace: "ns123",
		},
		Spec: bsv1.BackstageSpec{
			Application: &bsv1.Application{
				ExtraEnvs: &bsv1.ExtraEnvs{
					ConfigMaps: []bsv1.EnvObjectRef{},
				},
			},
		},
	}

	bs.Spec.Application.ExtraEnvs.ConfigMaps = append(bs.Spec.Application.ExtraEnvs.ConfigMaps,
		bsv1.EnvObjectRef{Name: "mapName", Key: "ENV1"})

	testObj := createBackstageTest(bs).withDefaultConfig(true)
	testObj.externalConfig.ExtraEnvConfigMaps["mapName"] = corev1.ConfigMap{Data: map[string]string{"mapName": "ENV1"}}

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, false, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	bscontainer := model.backstageDeployment.container()
	assert.NotNil(t, bscontainer)
	assert.Equal(t, 1, len(bscontainer.Env))

	assert.NotNil(t, bscontainer.Env[0])
	assert.Equal(t, "ENV1", bscontainer.Env[0].ValueFrom.ConfigMapKeyRef.Key)

}
