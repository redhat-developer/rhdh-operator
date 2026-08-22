package model

import (
	"context"
	"testing"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/platform"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"github.com/stretchr/testify/assert"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var dbNetworkPolicyTestBackstage = api.Backstage{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test-db-netpol",
		Namespace: "test-ns",
	},
}

func TestDefaultDbNetworkPolicies(t *testing.T) {
	bs := *dbNetworkPolicyTestBackstage.DeepCopy()
	testObj := createBackstageTest(bs).withDefaultConfig(true).withLocalDb(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model)

	obj := model.GetRuntimeObject(DbNetworkPolicyKey)
	assert.NotNil(t, obj)

	mo := obj.Object().(*multiobject.MultiObject)
	assert.Equal(t, 3, len(mo.Items))

	dbLabel := utils.BackstageDbAppLabelValue(bs.Name)
	for _, item := range mo.Items {
		np := item.(*networkingv1.NetworkPolicy)
		assert.Equal(t, dbLabel, np.Spec.PodSelector.MatchLabels[BackstageAppLabel])
		assert.Equal(t, bs.Namespace, np.GetNamespace())
	}
}

func TestDbNetworkPolicyDisabledWhenNoLocalDb(t *testing.T) {
	bs := *dbNetworkPolicyTestBackstage.DeepCopy()
	testObj := createBackstageTest(bs).withDefaultConfig(true).withLocalDb(false)

	model, err := InitObjects(context.TODO(), testObj.backstage, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model)

	obj := model.GetRuntimeObject(DbNetworkPolicyKey)
	assert.Nil(t, obj, "DB NetworkPolicies should not be created when localDb is disabled")
}

func TestDbNetworkPolicyPodSelectors(t *testing.T) {
	bs := *dbNetworkPolicyTestBackstage.DeepCopy()
	testObj := createBackstageTest(bs).withDefaultConfig(true).withLocalDb(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)

	mo := model.GetRuntimeObject(DbNetworkPolicyKey).Object().(*multiobject.MultiObject)
	backendLabel := utils.BackstageAppLabelValue(bs.Name)

	found := false
	for _, item := range mo.Items {
		np := item.(*networkingv1.NetworkPolicy)
		for _, ingress := range np.Spec.Ingress {
			for _, from := range ingress.From {
				if from.PodSelector != nil {
					if val, ok := from.PodSelector.MatchLabels[BackstageAppLabel]; ok {
						assert.Equal(t, backendLabel, val)
						found = true
					}
				}
			}
		}
	}
	assert.True(t, found, "expected to find a backend ingress rule in DB policies")
}

func TestDbNetworkPolicyNaming(t *testing.T) {
	bs := *dbNetworkPolicyTestBackstage.DeepCopy()
	testObj := createBackstageTest(bs).withDefaultConfig(true).withLocalDb(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)

	mo := model.GetRuntimeObject(DbNetworkPolicyKey).Object().(*multiobject.MultiObject)
	assert.Equal(t, DefaultMultiObjectName("db-netpol", bs.Name, "default-deny"), mo.Items[0].GetName())
	assert.Equal(t, "default-deny", mo.Items[0].GetAnnotations()[ConfiguredNameAnnotation])
}
