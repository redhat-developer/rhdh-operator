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

var networkPolicyTestBackstage = api.Backstage{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test-netpol",
		Namespace: "test-ns",
	},
}

func TestDefaultNetworkPolicies(t *testing.T) {
	bs := *networkPolicyTestBackstage.DeepCopy()
	testObj := createBackstageTest(bs).withDefaultConfig(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model)

	obj := model.GetRuntimeObject(NetworkPolicyKey)
	assert.NotNil(t, obj)

	mo := obj.Object().(*multiobject.MultiObject)
	assert.Equal(t, 6, len(mo.Items))

	backendLabel := utils.BackstageAppLabelValue(bs.Name)
	for _, item := range mo.Items {
		np := item.(*networkingv1.NetworkPolicy)
		assert.Equal(t, backendLabel, np.Spec.PodSelector.MatchLabels[BackstageAppLabel])
		assert.Equal(t, bs.Namespace, np.GetNamespace())
	}
}

func TestNetworkPolicyPodSelectors(t *testing.T) {
	bs := *networkPolicyTestBackstage.DeepCopy()
	testObj := createBackstageTest(bs).withDefaultConfig(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)

	mo := model.GetRuntimeObject(NetworkPolicyKey).Object().(*multiobject.MultiObject)
	dbLabel := utils.BackstageDbAppLabelValue(bs.Name)

	found := false
	for _, item := range mo.Items {
		np := item.(*networkingv1.NetworkPolicy)
		for _, egress := range np.Spec.Egress {
			for _, to := range egress.To {
				if to.PodSelector != nil {
					if val, ok := to.PodSelector.MatchLabels[BackstageAppLabel]; ok {
						assert.Equal(t, dbLabel, val)
						found = true
					}
				}
			}
		}
	}
	assert.True(t, found, "expected to find a psql egress rule targeting DB-labeled pods")
}

func TestNetworkPolicyNaming(t *testing.T) {
	bs := *networkPolicyTestBackstage.DeepCopy()
	testObj := createBackstageTest(bs).withDefaultConfig(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)

	mo := model.GetRuntimeObject(NetworkPolicyKey).Object().(*multiobject.MultiObject)
	assert.Equal(t, DefaultMultiObjectName("netpol", bs.Name, "default-deny"), mo.Items[0].GetName())
	assert.Equal(t, "default-deny", mo.Items[0].GetAnnotations()[ConfiguredNameAnnotation])
}

func TestNetworkPolicyRouterIngressOnOpenShift(t *testing.T) {
	bs := *networkPolicyTestBackstage.DeepCopy()
	testObj := createBackstageTest(bs).withDefaultConfig(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)

	mo := model.GetRuntimeObject(NetworkPolicyKey).Object().(*multiobject.MultiObject)

	var routerPolicy *networkingv1.NetworkPolicy
	for _, item := range mo.Items {
		np := item.(*networkingv1.NetworkPolicy)
		if np.GetAnnotations()[ConfiguredNameAnnotation] == "allow-router-ingress" {
			routerPolicy = np
			break
		}
	}
	assert.NotNil(t, routerPolicy, "expected allow-router-ingress policy")
	assert.Len(t, routerPolicy.Spec.Ingress, 1)
	assert.Len(t, routerPolicy.Spec.Ingress[0].From, 1)
	assert.NotNil(t, routerPolicy.Spec.Ingress[0].From[0].NamespaceSelector)
	assert.Equal(t, "", routerPolicy.Spec.Ingress[0].From[0].NamespaceSelector.MatchLabels["policy-group.network.openshift.io/ingress"])
}

func TestNetworkPolicyRouterIngressOnKubernetes(t *testing.T) {
	bs := *networkPolicyTestBackstage.DeepCopy()
	testObj := createBackstageTest(bs).withDefaultConfig(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)

	mo := model.GetRuntimeObject(NetworkPolicyKey).Object().(*multiobject.MultiObject)

	var routerPolicy *networkingv1.NetworkPolicy
	for _, item := range mo.Items {
		np := item.(*networkingv1.NetworkPolicy)
		if np.GetAnnotations()[ConfiguredNameAnnotation] == "allow-router-ingress" {
			routerPolicy = np
			break
		}
	}
	assert.NotNil(t, routerPolicy, "expected allow-router-ingress policy")
	assert.Len(t, routerPolicy.Spec.Ingress, 1)
	assert.Len(t, routerPolicy.Spec.Ingress[0].From, 1)
	ns := routerPolicy.Spec.Ingress[0].From[0].NamespaceSelector
	assert.NotNil(t, ns)
	assert.Empty(t, ns.MatchLabels, "non-OCP should use empty namespaceSelector (match all)")
}

func TestNetworkPolicyWithOverlay(t *testing.T) {
	bs := *networkPolicyTestBackstage.DeepCopy()
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("networkpolicy.yaml", "raw-networkpolicy.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)

	obj := model.GetRuntimeObject(NetworkPolicyKey)
	assert.NotNil(t, obj)

	mo := obj.Object().(*multiobject.MultiObject)
	assert.Equal(t, 1, len(mo.Items))
}
