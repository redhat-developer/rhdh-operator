package controller

import (
	"context"
	"testing"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	bs "github.com/redhat-developer/rhdh-operator/api/v1alpha4"
	"github.com/stretchr/testify/assert"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func setupMonitorTestReconciler() BackstageReconciler {
	scheme := runtime.NewScheme()
	_ = bs.AddToScheme(scheme)
	_ = monitoringv1.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)

	return BackstageReconciler{
		Client: NewMockClient(),
		Scheme: scheme,
	}
}

func createTestBackstage(name, namespace string, monitoringEnabled bool) *bs.Backstage {
	backstage := &bs.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: bs.BackstageSpec{
			Monitoring: bs.Monitoring{
				Enabled: monitoringEnabled,
			},
		},
	}

	return backstage
}

func TestApplyServiceMonitor_MonitoringDisabled(t *testing.T) {
	ctx := context.TODO()
	r := setupMonitorTestReconciler()

	backstage := createTestBackstage("test-bs", "test-ns", false)

	// Create the backstage object
	err := r.Create(ctx, backstage)
	assert.NoError(t, err)

	// Create an existing ServiceMonitor to test deletion
	existingSM := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backstage.Name + "-metrics",
			Namespace: backstage.Namespace,
		},
		Spec: monitoringv1.ServiceMonitorSpec{},
	}
	err = r.Create(ctx, existingSM)
	assert.NoError(t, err)

	// Apply service monitor (should delete the existing one)
	err = r.applyServiceMonitor(ctx, backstage)
	assert.NoError(t, err)

	// Verify ServiceMonitor was deleted
	sm := &monitoringv1.ServiceMonitor{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      backstage.Name + "-metrics",
		Namespace: backstage.Namespace,
	}, sm)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestApplyServiceMonitor_MonitoringEnabled_NoCRD(t *testing.T) {
	ctx := context.TODO()
	r := setupMonitorTestReconciler()

	backstage := createTestBackstage("test-bs", "test-ns", true)

	// Create the backstage object
	err := r.Create(ctx, backstage)
	assert.NoError(t, err)

	// Apply service monitor (should fail due to missing CRD)
	err = r.applyServiceMonitor(ctx, backstage)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ServiceMonitor CRD not found")
}

func TestApplyServiceMonitor_MonitoringEnabled_WithCRD(t *testing.T) {
	ctx := context.TODO()
	r := setupMonitorTestReconciler()

	backstage := createTestBackstage("test-bs", "test-ns", true)

	// Create the backstage object
	err := r.Create(ctx, backstage)
	assert.NoError(t, err)

	// Create ServiceMonitor CRD
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "servicemonitors.monitoring.coreos.com",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "monitoring.coreos.com",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
						},
					},
				},
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural: "servicemonitors",
				Kind:   "ServiceMonitor",
			},
		},
	}
	err = r.Create(ctx, crd)
	assert.NoError(t, err)

	// Apply service monitor (should succeed)
	err = r.applyServiceMonitor(ctx, backstage)
	assert.NoError(t, err)

	// Verify ServiceMonitor was created
	sm := &monitoringv1.ServiceMonitor{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      backstage.Name + "-metrics",
		Namespace: backstage.Namespace,
	}, sm)
	assert.NoError(t, err)

	// Verify ServiceMonitor configuration
	assert.Equal(t, backstage.Name+"-metrics", sm.Name)
	assert.Equal(t, backstage.Namespace, sm.Namespace)
	assert.Equal(t, "monitoring.coreos.com/v1", sm.TypeMeta.APIVersion)
	assert.Equal(t, "ServiceMonitor", sm.TypeMeta.Kind)

	// Verify labels
	expectedLabels := map[string]string{
		"app.kubernetes.io/instance": backstage.Name,
		"app.kubernetes.io/name":     "backstage",
	}
	assert.Equal(t, expectedLabels, sm.Labels)

	// Verify spec
	assert.Equal(t, expectedLabels, sm.Spec.Selector.MatchLabels)
	assert.Equal(t, []string{backstage.Namespace}, sm.Spec.NamespaceSelector.MatchNames)
	assert.Len(t, sm.Spec.Endpoints, 1)
	assert.Equal(t, "http-metrics", sm.Spec.Endpoints[0].Port)
	assert.Equal(t, "/metrics", sm.Spec.Endpoints[0].Path)

	// Verify controller reference is set
	assert.Len(t, sm.OwnerReferences, 1)
	assert.Equal(t, backstage.Name, sm.OwnerReferences[0].Name)
	assert.Equal(t, "Backstage", sm.OwnerReferences[0].Kind)
}

func TestApplyServiceMonitor_Update_ExistingServiceMonitor(t *testing.T) {
	ctx := context.TODO()
	r := setupMonitorTestReconciler()

	backstage := createTestBackstage("test-bs", "test-ns", true)

	// Create the backstage object
	err := r.Create(ctx, backstage)
	assert.NoError(t, err)

	// Create ServiceMonitor CRD
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "servicemonitors.monitoring.coreos.com",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "monitoring.coreos.com",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
						},
					},
				},
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural: "servicemonitors",
				Kind:   "ServiceMonitor",
			},
		},
	}
	err = r.Create(ctx, crd)
	assert.NoError(t, err)

	// Create an existing ServiceMonitor with different configuration
	existingSM := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backstage.Name + "-metrics",
			Namespace: backstage.Namespace,
			Labels: map[string]string{
				"old-label": "old-value",
			},
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Endpoints: []monitoringv1.Endpoint{
				{
					Port:     "old-port",
					Path:     "/old-metrics",
					Interval: "60s",
				},
			},
		},
	}
	err = r.Create(ctx, existingSM)
	assert.NoError(t, err)

	// Apply service monitor (should update the existing one)
	err = r.applyServiceMonitor(ctx, backstage)
	assert.NoError(t, err)

	// Verify ServiceMonitor was updated
	sm := &monitoringv1.ServiceMonitor{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      backstage.Name + "-metrics",
		Namespace: backstage.Namespace,
	}, sm)
	assert.NoError(t, err)

	// Verify updated configuration
	expectedLabels := map[string]string{
		"app.kubernetes.io/instance": backstage.Name,
		"app.kubernetes.io/name":     "backstage",
	}
	assert.Equal(t, expectedLabels, sm.Labels)
	assert.Equal(t, "http-metrics", sm.Spec.Endpoints[0].Port)
	assert.Equal(t, "/metrics", sm.Spec.Endpoints[0].Path)
}

func TestServiceMonitorCRDExists(t *testing.T) {
	ctx := context.TODO()
	r := setupMonitorTestReconciler()

	// Test when CRD doesn't exist
	exists := r.serviceMonitorCRDExists(ctx)
	assert.False(t, exists)

	// Create the CRD
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "servicemonitors.monitoring.coreos.com",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "monitoring.coreos.com",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
						},
					},
				},
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural: "servicemonitors",
				Kind:   "ServiceMonitor",
			},
		},
	}
	err := r.Create(ctx, crd)
	assert.NoError(t, err)

	// Test when CRD exists
	exists = r.serviceMonitorCRDExists(ctx)
	assert.True(t, exists)
}

// Test edge case where client returns an error other than NotFound
func TestServiceMonitorCRDExists_ClientError(t *testing.T) {
	ctx := context.TODO()

	// Create a mock client that returns an error
	scheme := runtime.NewScheme()
	_ = apiextensionsv1.AddToScheme(scheme)

	mockClient := &mockErrorClient{
		Client: NewMockClient(),
	}

	r := BackstageReconciler{
		Client: mockClient,
		Scheme: scheme,
	}

	// Should return false on client errors
	exists := r.serviceMonitorCRDExists(ctx)
	assert.False(t, exists)
}

// mockErrorClient is a mock client that returns errors for Get operations
type mockErrorClient struct {
	client.Client
}

func (m *mockErrorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*apiextensionsv1.CustomResourceDefinition); ok {
		return assert.AnError // Return a generic error
	}
	return m.Client.Get(ctx, key, obj, opts...)
}
