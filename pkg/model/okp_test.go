package model

import (
	"testing"

	"github.com/stretchr/testify/assert"

	openshift "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/redhat-developer/rhdh-operator/api"
)

func okpTestBackstage() api.Backstage {
	return api.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-bs",
			Namespace: "my-ns",
		},
	}
}

func TestOkpName(t *testing.T) {
	assert.Equal(t, "lightspeed-okp-my-bs", OkpName("my-bs"))
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

func TestMergeOkpObjectEmptySources(t *testing.T) {
	objs, err := mergeOkpObject(nil, *runtime.NewScheme(), "")
	assert.NoError(t, err)
	assert.Empty(t, objs)
}

// okpTestScheme returns a scheme with all types the OKP objects need for
// SetControllerReference during addToModel.
func okpTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := createBackstageTest(okpTestBackstage()).scheme
	return scheme
}

func TestOkpDeploymentAddToModelOpenShift(t *testing.T) {
	bs := okpTestBackstage()
	scheme := okpTestScheme(t)
	m := &BackstageModel{isOpenshift: true}

	config := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "okp", Image: "registry.example.com/okp:1.0"}},
				},
			},
		},
	}

	o := &OkpDeployment{}
	assert.NoError(t, o.addToModel(m, bs, config, scheme))

	assert.NotNil(t, o.Object())
	assert.Equal(t, OkpName("my-bs"), o.deployment.Name)
	assert.Equal(t, okpSelectorLabels("my-bs"), o.deployment.Spec.Selector.MatchLabels)
	assert.Equal(t, okpSelectorLabels("my-bs"), o.deployment.Spec.Template.Labels)
	// Object added to the model regardless (placeholder pattern).
	assert.Len(t, m.RuntimeObjects, 1)
}

func TestOkpDeploymentAddToModelKubernetesSkipped(t *testing.T) {
	bs := okpTestBackstage()
	scheme := okpTestScheme(t)
	m := &BackstageModel{isOpenshift: false}

	config := &appsv1.Deployment{}
	o := &OkpDeployment{}
	assert.NoError(t, o.addToModel(m, bs, config, scheme))

	// OKP is OpenShift-only: object dropped on vanilla K8s.
	assert.Nil(t, o.Object())
	// Wrapper is still added, but Object() returns nil so it is not applied.
	assert.Len(t, m.RuntimeObjects, 1)
	assert.Nil(t, m.GetRuntimeObject(OkpDeploymentKey))
}

func TestOkpDeploymentAddToModelFlavourDisabled(t *testing.T) {
	bs := okpTestBackstage()
	scheme := okpTestScheme(t)
	m := &BackstageModel{isOpenshift: true}

	// Flavour disabled => no config sourced from the flavour dir.
	o := &OkpDeployment{}
	assert.NoError(t, o.addToModel(m, bs, nil, scheme))

	assert.Nil(t, o.Object())
}

func TestOkpServiceAddToModel(t *testing.T) {
	bs := okpTestBackstage()
	scheme := okpTestScheme(t)

	// OpenShift: object created with selector wired to OKP pods.
	m := &BackstageModel{isOpenshift: true}
	config := &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP}}
	o := &OkpService{}
	assert.NoError(t, o.addToModel(m, bs, config, scheme))
	assert.NotNil(t, o.Object())
	assert.Equal(t, OkpName("my-bs"), o.service.Name)
	assert.Equal(t, okpSelectorLabels("my-bs"), o.service.Spec.Selector)

	// Kubernetes: skipped.
	mK8s := &BackstageModel{isOpenshift: false}
	oK8s := &OkpService{}
	assert.NoError(t, oK8s.addToModel(mK8s, bs, &corev1.Service{}, scheme))
	assert.Nil(t, oK8s.Object())
}

func TestOkpRouteAddToModel(t *testing.T) {
	bs := okpTestBackstage()
	scheme := okpTestScheme(t)

	// OpenShift: Route created, pointing at the OKP Service (same name).
	m := &BackstageModel{isOpenshift: true}
	config := &openshift.Route{Spec: openshift.RouteSpec{To: openshift.RouteTargetReference{Kind: "Service"}}}
	o := &OkpRoute{}
	assert.NoError(t, o.addToModel(m, bs, config, scheme))
	assert.NotNil(t, o.Object())
	assert.Equal(t, OkpName("my-bs"), o.route.Name)
	assert.Equal(t, OkpName("my-bs"), o.route.Spec.To.Name)

	// Kubernetes: skipped (Route is OpenShift-only anyway).
	mK8s := &BackstageModel{isOpenshift: false}
	oK8s := &OkpRoute{}
	assert.NoError(t, oK8s.addToModel(mK8s, bs, &openshift.Route{}, scheme))
	assert.Nil(t, oK8s.Object())
}
