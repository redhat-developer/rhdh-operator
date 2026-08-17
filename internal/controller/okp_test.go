package controller

import (
	"context"
	"testing"

	openshift "github.com/openshift/api/route/v1"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model"
	"github.com/redhat-developer/rhdh-operator/pkg/platform"
)

// okpTestFlavoursDir points LOCALBIN at a testdata tree containing a
// intelligent-assistant flavour with enabledByDefault: true.
const okpTestFlavoursDir = "testdata/okpflavours"

func setupOkpReconciler(p platform.Platform) BackstageReconciler {
	scheme := runtime.NewScheme()
	_ = api.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = openshift.AddToScheme(scheme)

	return BackstageReconciler{
		Client:   NewMockClient(),
		Scheme:   scheme,
		Platform: p,
	}
}

func okpTestBackstage(name, namespace string, flavours *[]api.Flavour) *api.Backstage {
	return &api.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: api.BackstageSpec{
			Flavours: flavours,
		},
	}
}

func TestOkpName(t *testing.T) {
	name := okpName("my-bs")
	assert.Contains(t, name, "my-bs")
	assert.Contains(t, name, okpComponentName)
}

func TestOkpLabels(t *testing.T) {
	labels := okpLabels("my-bs")
	assert.Equal(t, okpComponentName, labels["app.kubernetes.io/name"])
	assert.Equal(t, okpComponentName, labels["app.kubernetes.io/component"])
	assert.Equal(t, "my-bs", labels["app.kubernetes.io/instance"])
}

func TestOkpSelectorLabels(t *testing.T) {
	labels := okpLabels("my-bs")
	selector := okpSelectorLabels("my-bs")

	// Every selector label must be a subset of the object labels, otherwise
	// the Deployment/Service selector would not match the pods.
	for k, v := range selector {
		assert.Equal(t, v, labels[k], "selector label %q must match object label", k)
	}
	// The component label is intentionally NOT part of the selector.
	assert.NotContains(t, selector, "app.kubernetes.io/component")
}

func TestOkpServiceURL(t *testing.T) {
	// In-cluster Service DNS when no ingress domain is known.
	assert.Equal(t,
		"http://okp.ns.svc.cluster.local:8080",
		okpServiceURL("okp", "ns", ""))

	// Route hostname when an OpenShift ingress domain is known.
	assert.Equal(t,
		"http://okp-ns.apps.example.com",
		okpServiceURL("okp", "ns", "apps.example.com"))
}

func TestApplyOkpResourcesFlavourDisabled(t *testing.T) {
	ctx := context.TODO()
	r := setupOkpReconciler(platform.OpenShift)

	// Explicit empty flavours array disables everything without touching disk.
	bs := okpTestBackstage("test-bs", "test-ns", &[]api.Flavour{})

	err := r.applyOkpResources(ctx, bs, &model.BackstageModel{})
	assert.NoError(t, err)

	// No OKP objects should have been applied.
	assert.Error(t, r.Get(ctx, types.NamespacedName{Name: okpName("test-bs"), Namespace: "test-ns"}, &appsv1.Deployment{}))
}

func TestApplyOkpResourcesKubernetesSkipped(t *testing.T) {
	t.Setenv("LOCALBIN", okpTestFlavoursDir)

	ctx := context.TODO()
	r := setupOkpReconciler(platform.Kubernetes)

	// Lightspeed enabled by default (via testdata), but not on OpenShift.
	bs := okpTestBackstage("test-bs", "test-ns", nil)
	assert.True(t, model.IsFlavourEnabled(bs.Spec, "intelligent-assistant"))

	err := r.applyOkpResources(ctx, bs, &model.BackstageModel{})
	assert.NoError(t, err)

	// OKP is OpenShift-only: nothing should be applied on vanilla K8s.
	assert.Error(t, r.Get(ctx, types.NamespacedName{Name: okpName("test-bs"), Namespace: "test-ns"}, &appsv1.Deployment{}))
	assert.Error(t, r.Get(ctx, types.NamespacedName{Name: okpName("test-bs"), Namespace: "test-ns"}, &corev1.Service{}))
}

func TestApplyOkpResourcesOpenShift(t *testing.T) {
	t.Setenv("LOCALBIN", okpTestFlavoursDir)

	ctx := context.TODO()
	r := setupOkpReconciler(platform.OpenShift)

	bs := okpTestBackstage("test-bs", "test-ns", nil)
	assert.True(t, model.IsFlavourEnabled(bs.Spec, "intelligent-assistant"))

	err := r.applyOkpResources(ctx, bs, &model.BackstageModel{})
	assert.NoError(t, err)

	name := okpName("test-bs")
	key := types.NamespacedName{Name: name, Namespace: "test-ns"}

	// Deployment applied with the expected OKP image and both ports.
	deployment := &appsv1.Deployment{}
	assert.NoError(t, r.Get(ctx, key, deployment))
	assert.Len(t, deployment.Spec.Template.Spec.Containers, 1)
	c := deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "registry.redhat.io/offline-knowledge-portal/rhokp-rhel9:1.2.10-1786628394", c.Image)
	assert.Len(t, c.Ports, 2)

	// Service applied as ClusterIP.
	service := &corev1.Service{}
	assert.NoError(t, r.Get(ctx, key, service))
	assert.Equal(t, corev1.ServiceTypeClusterIP, service.Spec.Type)

	// Route applied (OpenShift only).
	route := &openshift.Route{}
	assert.NoError(t, r.Get(ctx, key, route))
	assert.Equal(t, name, route.Spec.To.Name)
}
