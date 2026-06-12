package model

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/platform"

	"k8s.io/utils/ptr"

	"github.com/redhat-developer/rhdh-operator/api"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

//func TestIfEmptyObjectsContainTypeinfo(t *testing.T) {
//	for _, cfg := range runtimeConfig {
//		cfg.ObjectFactory.newBackstageObject()
//		//assert.NotNil(t, Obj.EmptyObject())
//		// TODO uncomment when Kind is available
//		//assert.NotEmpty(t, Obj.EmptyObject().GetObjectKind().GroupVersionKind().Kind)
//	}
//}

// NOTE: to make it work locally env var LOCALBIN should point to the directory where default-config folder located
func TestInitDefaultDeploy(t *testing.T) {

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

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	bsDeployment := model.GetRuntimeObject(DeploymentKey).(*BackstageDeployment)

	assert.NoError(t, err)
	assert.True(t, len(model.GetRuntimeObjects()) > 0)
	assert.Equal(t, DeploymentName(bs.Name), bsDeployment.deployable.GetObject().GetName())
	assert.Equal(t, "ns123", bsDeployment.deployable.GetObject().GetNamespace())
	assert.Equal(t, 2, len(bsDeployment.deployable.GetObject().GetLabels()))

	assert.NotNil(t, bsDeployment.container())

	bsService := model.GetRuntimeObject(ServiceKey).(*BackstageService)
	assert.Equal(t, ServiceName(bs.Name), bsService.service.Name)
	assert.True(t, len(bsService.service.Spec.Ports) > 0)

	assert.Equal(t, fmt.Sprintf("backstage-%s", "bs"), bsDeployment.deployable.PodObjectMeta().Labels[BackstageAppLabel])
	assert.Equal(t, fmt.Sprintf("backstage-%s", "bs"), bsService.service.Spec.Selector[BackstageAppLabel])

}

func TestIfEmptyObjectIsValid(t *testing.T) {

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

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	assert.False(t, bs.Spec.IsLocalDbEnabled())

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)

	t.Logf("model.localDbEnabled: %v, model.isOpenshift: %v", model.localDbEnabled, model.isOpenshift)

	// With the new slice-based API, only objects to be applied are in the slice
	objs := model.GetRuntimeObjects()
	for i, obj := range objs {
		t.Logf("Object at index %d, Object(): %v", i, obj.Object())
	}

	// Debug db-service
	dbSvc := model.GetRuntimeObject(DbServiceKey)
	if dbSvc != nil {
		t.Logf("DbService wrapper exists, service: %v, Object(): %v", dbSvc.(*DbService).service, dbSvc.Object())
	}

	assert.Equal(t, 2, len(objs), "Should have 2 objects to apply (deployment + service)")

}

func TestAddToModel(t *testing.T) {

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
	testObj := createBackstageTest(bs).withDefaultConfig(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model)
	assert.NotNil(t, model.GetRuntimeObjects())

	// With the new map-based API, only objects to be applied are in the map
	assert.Equal(t, 2, len(model.GetRuntimeObjects()), "Should have 2 objects to apply (deployment + service)")

	// Verify deployment is in the map
	deployment := model.GetRuntimeObject(DeploymentKey)
	assert.NotNil(t, deployment, "Deployment should be in the map")
	assert.IsType(t, &BackstageDeployment{}, deployment)

	// another empty model to test
	rm := BackstageModel{}
	assert.Equal(t, 0, len(rm.GetRuntimeObjects()))
	testService := *model.GetRuntimeObject(ServiceKey).(*BackstageService)

	// add to rm
	err = testService.addToModel(&rm, bs, testService.service, testObj.scheme)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(rm.GetRuntimeObjects()))
	assert.NotNil(t, rm.GetRuntimeObject(ServiceKey))
	assert.Nil(t, rm.GetRuntimeObject(DeploymentKey))
	assert.Equal(t, testService, *rm.GetRuntimeObject(ServiceKey).(*BackstageService))
}

func TestRawConfig(t *testing.T) {
	bs := api.Backstage{
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
    raw: "true"
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
	assert.NotNil(t, model.GetRuntimeObject(ServiceKey).(*BackstageService))
	assert.Equal(t, "true", model.GetRuntimeObject(ServiceKey).(*BackstageService).service.GetLabels()["default"])
	assert.Empty(t, model.GetRuntimeObject(ServiceKey).(*BackstageService).service.GetLabels()["raw"])

	// Put raw config
	model, err = InitObjects(context.TODO(), bs, extConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model.GetRuntimeObject(ServiceKey).(*BackstageService))
	assert.Equal(t, "true", model.GetRuntimeObject(ServiceKey).(*BackstageService).service.GetLabels()["raw"])
	assert.Empty(t, model.GetRuntimeObject(ServiceKey).(*BackstageService).service.GetLabels()["default"])
}

func TestMultiobject(t *testing.T) {
	bs := api.Backstage{}
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("pvcs.yaml", "multi-pvc.yaml")
	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model)
	pvcs := model.GetRuntimeObject(PvcsKey)
	assert.NotNil(t, pvcs, "PVCs should be in the map")
	backstagePvcs, ok := pvcs.(*BackstagePvcs)
	assert.True(t, ok)
	obj := backstagePvcs.Object()
	items := obj.(*multiobject.MultiObject).Items
	assert.Equal(t, 2, len(items))
}

func TestSingleMultiobject(t *testing.T) {
	bs := api.Backstage{}
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("pvcs.yaml", "single-pvc.yaml")
	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model)
	pvcs := model.GetRuntimeObject(PvcsKey)
	assert.NotNil(t, pvcs, "PVCs should be in the map")
	backstagePvcs, ok := pvcs.(*BackstagePvcs)
	assert.True(t, ok)
	obj := backstagePvcs.Object()
	items := obj.(*multiobject.MultiObject).Items
	assert.Equal(t, 1, len(items))
}

func TestSingleFailedWithMultiDefinition(t *testing.T) {
	bs := api.Backstage{}
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("service.yaml", "multi-service-err.yaml")
	_, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	// Error message can be from overlay or default
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "multiple objects not expected for: service.yaml")
}

func TestInvalidObjectKind(t *testing.T) {
	bs := api.Backstage{}
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("service.yaml", "invalid-service-type.yaml")
	_, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	// Error can come from either overlay or default config reading
	assert.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "failed to read overlay config for the key service.yaml") ||
			strings.Contains(err.Error(), "failed to read default value for the key service.yaml"),
		"Error should mention failed to read config for service.yaml")
}
