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
	testObj := createBackstageTest(bs).withDefaultConfig(true).withLocalDb(true)

	model, err := InitObjects(context.TODO(), testObj.backstage, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model)

	obj := model.GetRuntimeObject(NetworkPolicyKey)
	assert.NotNil(t, obj)

	mo := obj.Object().(*multiobject.MultiObject)
	assert.Equal(t, 9, len(mo.Items), "expected 6 backend + 3 DB NPs")

	backendLabel := utils.BackstageAppLabelValue(bs.Name)
	dbLabel := utils.BackstageDbAppLabelValue(bs.Name)
	var backendCount, dbCount int
	for _, item := range mo.Items {
		np := item.(*networkingv1.NetworkPolicy)
		assert.Equal(t, bs.Namespace, np.GetNamespace())
		if np.Spec.PodSelector.MatchLabels[BackstageAppLabel] == dbLabel {
			dbCount++
		} else {
			assert.Equal(t, backendLabel, np.Spec.PodSelector.MatchLabels[BackstageAppLabel])
			backendCount++
		}
	}
	assert.Equal(t, 6, backendCount, "expected 6 backend NPs")
	assert.Equal(t, 3, dbCount, "expected 3 DB NPs")
}

func TestNetworkPoliciesWithoutLocalDb(t *testing.T) {
	bs := *networkPolicyTestBackstage.DeepCopy()
	testObj := createBackstageTest(bs).withDefaultConfig(true).withLocalDb(false)

	model, err := InitObjects(context.TODO(), testObj.backstage, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)

	obj := model.GetRuntimeObject(NetworkPolicyKey)
	assert.NotNil(t, obj)

	mo := obj.Object().(*multiobject.MultiObject)
	assert.Equal(t, 6, len(mo.Items), "expected only 6 backend NPs when localDb is disabled")

	backendLabel := utils.BackstageAppLabelValue(testObj.backstage.Name)
	for _, item := range mo.Items {
		np := item.(*networkingv1.NetworkPolicy)
		assert.Equal(t, backendLabel, np.Spec.PodSelector.MatchLabels[BackstageAppLabel],
			"all NPs should have backend label when localDb is disabled")
	}

	var psqlEgress *networkingv1.NetworkPolicy
	for _, item := range mo.Items {
		np := item.(*networkingv1.NetworkPolicy)
		if np.GetAnnotations()[ConfiguredNameAnnotation] == "allow-psql-egress" {
			psqlEgress = np
			break
		}
	}
	assert.NotNil(t, psqlEgress, "allow-psql-egress should still exist when localDb is disabled")
	assert.Len(t, psqlEgress.Spec.Egress, 1)
	assert.Nil(t, psqlEgress.Spec.Egress[0].To,
		"psql egress should allow port 5432 to any destination when localDb is disabled")
}

func findPeerLabel(peers []networkingv1.NetworkPolicyPeer) (string, bool) {
	for _, peer := range peers {
		if peer.PodSelector == nil {
			continue
		}
		if val, ok := peer.PodSelector.MatchLabels[BackstageAppLabel]; ok {
			return val, true
		}
	}
	return "", false
}

func TestNetworkPolicyPodSelectors(t *testing.T) {
	bs := *networkPolicyTestBackstage.DeepCopy()
	testObj := createBackstageTest(bs).withDefaultConfig(true).withLocalDb(true)

	model, err := InitObjects(context.TODO(), testObj.backstage, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)

	mo := model.GetRuntimeObject(NetworkPolicyKey).Object().(*multiobject.MultiObject)
	dbLabel := utils.BackstageDbAppLabelValue(bs.Name)
	backendLabel := utils.BackstageAppLabelValue(bs.Name)

	var foundEgressToDb, foundIngressFromBackend bool
	for _, item := range mo.Items {
		np := item.(*networkingv1.NetworkPolicy)
		for _, egress := range np.Spec.Egress {
			if val, ok := findPeerLabel(egress.To); ok {
				assert.Equal(t, dbLabel, val)
				foundEgressToDb = true
			}
		}
		for _, ingress := range np.Spec.Ingress {
			if val, ok := findPeerLabel(ingress.From); ok {
				assert.Equal(t, backendLabel, val)
				foundIngressFromBackend = true
			}
		}
	}
	assert.True(t, foundEgressToDb, "expected to find a psql egress rule targeting DB-labeled pods")
	assert.True(t, foundIngressFromBackend, "expected to find a backend ingress rule in DB policies")
}

func TestNetworkPolicyNaming(t *testing.T) {
	bs := *networkPolicyTestBackstage.DeepCopy()
	testObj := createBackstageTest(bs).withDefaultConfig(true).withLocalDb(true)

	model, err := InitObjects(context.TODO(), testObj.backstage, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)

	mo := model.GetRuntimeObject(NetworkPolicyKey).Object().(*multiobject.MultiObject)

	var foundBackendNaming, foundDbNaming bool
	for _, item := range mo.Items {
		np := item.(*networkingv1.NetworkPolicy)
		ann := np.GetAnnotations()[ConfiguredNameAnnotation]
		if ann == "default-deny" && np.GetName() == DefaultMultiObjectName("netpol", bs.Name, "default-deny") {
			foundBackendNaming = true
		}
		if ann == "db-default-deny" && np.GetName() == DefaultMultiObjectName("db-netpol", bs.Name, "db-default-deny") {
			foundDbNaming = true
		}
	}
	assert.True(t, foundBackendNaming, "backend NP should use 'netpol' name prefix")
	assert.True(t, foundDbNaming, "DB NP should use 'db-netpol' name prefix")
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

func TestNetworkPolicyFlavourMergePreservesNamespaceWidePodSelector(t *testing.T) {
	bs := api.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-flavour-np",
			Namespace: "test-ns",
		},
	}
	testObj := createBackstageTest(bs).withConfigPath("./testdata/testflavours").withLocalDb(false)

	model, err := InitObjects(context.TODO(), testObj.backstage, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)

	obj := model.GetRuntimeObject(NetworkPolicyKey)
	assert.NotNil(t, obj)

	mo := obj.Object().(*multiobject.MultiObject)
	assert.Equal(t, 7, len(mo.Items), "expected 6 backend + 1 flavour NP (DB NPs filtered out)")

	backendLabel := utils.BackstageAppLabelValue(testObj.backstage.Name)
	var foundNamespaceWide bool
	for _, item := range mo.Items {
		np := item.(*networkingv1.NetworkPolicy)
		if np.GetAnnotations()[ConfiguredNameAnnotation] == "allow-intra-network" &&
			np.GetAnnotations()[SourceAnnotation] == "flavour-flavor1" {
			foundNamespaceWide = true
			_, hasLabel := np.Spec.PodSelector.MatchLabels[BackstageAppLabel]
			assert.False(t, hasLabel, "flavour NP with podSelector: {} should NOT have rhdh.redhat.com/app in podSelector")
		} else {
			assert.Equal(t, backendLabel, np.Spec.PodSelector.MatchLabels[BackstageAppLabel],
				"base NP %s should have backend label in podSelector", np.GetAnnotations()[ConfiguredNameAnnotation])
		}
	}
	assert.True(t, foundNamespaceWide, "expected to find the namespace-wide flavour NP")
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
