package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/redhat-developer/rhdh-operator/pkg/platform"

	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"

	"k8s.io/utils/ptr"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

func TestIfEmptyObjectsContainTypeinfo(t *testing.T) {
	for _, cfg := range runtimeConfig {
		obj := cfg.ObjectFactory.newBackstageObject()
		assert.NotNil(t, obj.EmptyObject())
		// TODO uncomment when Kind is available
		//assert.NotEmpty(t, obj.EmptyObject().GetObjectKind().GroupVersionKind().Kind)
	}
}

// NOTE: to make it work locally env var LOCALBIN should point to the directory where default-config folder located
func TestInitDefaultDeploy(t *testing.T) {

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

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	bsDeployment := model.backstageDeployment

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)
	assert.Equal(t, DeploymentName(bs.Name), bsDeployment.deployment.GetName())
	assert.Equal(t, "ns123", bsDeployment.deployment.GetNamespace())
	assert.Equal(t, 2, len(bsDeployment.deployment.GetLabels()))

	assert.NotNil(t, bsDeployment.container())

	bsService := model.backstageService
	assert.Equal(t, ServiceName(bs.Name), bsService.service.Name)
	assert.True(t, len(bsService.service.Spec.Ports) > 0)

	assert.Equal(t, fmt.Sprintf("backstage-%s", "bs"), bsDeployment.deployment.Spec.Template.ObjectMeta.Labels[BackstageAppLabel])
	assert.Equal(t, fmt.Sprintf("backstage-%s", "bs"), bsService.service.Spec.Selector[BackstageAppLabel])

}

func TestIfEmptyObjectIsValid(t *testing.T) {

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

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	assert.False(t, bs.Spec.IsLocalDbEnabled())

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)

	assert.Equal(t, 2, len(model.RuntimeObjects))

}

func TestAddToModel(t *testing.T) {

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
	testObj := createBackstageTest(bs).withDefaultConfig(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model)
	assert.NotNil(t, model.RuntimeObjects)
	assert.Equal(t, 2, len(model.RuntimeObjects))

	found := false
	for _, bd := range model.RuntimeObjects {
		if bd, ok := bd.(*BackstageDeployment); ok {
			found = true
			assert.Equal(t, bd, model.backstageDeployment)
		}
	}
	assert.True(t, found)

	// another empty model to test
	rm := BackstageModel{RuntimeObjects: []RuntimeObject{}}
	assert.Equal(t, 0, len(rm.RuntimeObjects))
	testService := *model.backstageService

	// add to rm
	_, _ = testService.addToModel(&rm, bs)
	assert.Equal(t, 1, len(rm.RuntimeObjects))
	assert.NotNil(t, rm.backstageService)
	assert.Nil(t, rm.backstageDeployment)
	assert.Equal(t, testService, *rm.backstageService)
	assert.Equal(t, testService, *rm.RuntimeObjects[0].(*BackstageService))
}

func TestRawConfig(t *testing.T) {
	bs := bsv1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bs",
			Namespace: "ns123",
		},
	}
	testObj := createBackstageTest(bs).withDefaultConfig(true)
	serviceYaml := `apiVersion: v1
kind: Service
metadata:
 labels:
    raw: true
spec:
 ports:
   - port: 8080`

	extConfig := ExternalConfig{
		RawConfig: map[string]string{
			"service.yaml": serviceYaml,
		},
	}

	// No raw config
	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model.backstageService)
	assert.Equal(t, "true", model.backstageService.service.GetLabels()["default"])
	assert.Empty(t, model.backstageService.service.GetLabels()["raw"])

	// Put raw config
	model, err = InitObjects(context.TODO(), bs, extConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model.backstageService)
	assert.Equal(t, "true", model.backstageService.service.GetLabels()["raw"])
	assert.Empty(t, model.backstageService.service.GetLabels()["default"])
}

func TestMultiobject(t *testing.T) {
	bs := bsv1.Backstage{}
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("pvcs.yaml", "multi-pvc.yaml")
	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model)
	found := false
	for _, ro := range model.RuntimeObjects {
		if pvcs, ok := ro.(*BackstagePvcs); ok {
			items := pvcs.Object().(*multiobject.MultiObject).Items
			assert.Equal(t, 2, len(items))
			found = true
		}
	}
	assert.True(t, found)
}

func TestSingleMultiobject(t *testing.T) {
	bs := bsv1.Backstage{}
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("pvcs.yaml", "single-pvc.yaml")
	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model)
	found := false
	for _, ro := range model.RuntimeObjects {
		if pvcs, ok := ro.(*BackstagePvcs); ok {
			items := pvcs.Object().(*multiobject.MultiObject).Items
			assert.Equal(t, 1, len(items))
			found = true
		}
	}
	assert.True(t, found)
}

func TestSingleFailedWithMultiDefinition(t *testing.T) {
	bs := bsv1.Backstage{}
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("service.yaml", "multi-service-err.yaml")
	_, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.EqualError(t, err, "failed to initialize object: multiple objects not expected for: service.yaml")
}

func TestInvalidObjectKind(t *testing.T) {
	bs := bsv1.Backstage{}
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("service.yaml", "invalid-service-type.yaml")
	_, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.EqualError(t, err, "failed to read default value for the key service.yaml, reason: GroupVersionKind not match, found: /v1, Kind=Deployment, expected: [/v1, Kind=Service]")
}
