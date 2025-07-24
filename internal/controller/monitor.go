package controller

import (
	"context"
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	bs "github.com/redhat-developer/rhdh-operator/api/v1alpha4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (r *BackstageReconciler) applyServiceMonitor(ctx context.Context, backstage *bs.Backstage) error {
	lg := log.FromContext(ctx).WithValues("Backstage", backstage.Name)

	lg.Info("Checking monitoring status", "enabled", backstage.Spec.IsMonitoringEnabled())

	if !backstage.Spec.IsMonitoringEnabled() {
		lg.Info("Monitoring disabled, deleting any existing ServiceMonitor")
		return r.tryToDelete(ctx,
			&monitoringv1.ServiceMonitor{},
			backstage.Name+"-metrics",
			backstage.Namespace,
		)
	}

	// Check if ServiceMonitor CRD exists before creating
	if !r.serviceMonitorCRDExists(ctx) {
		lg.Error(nil, "Monitoring enabled but ServiceMonitor CRD not found. Please install Prometheus Operator")
		return fmt.Errorf("monitoring enabled but ServiceMonitor CRD not found. Please install Prometheus Operator")
	}

	lg.Info("Monitoring enabled, creating/updating ServiceMonitor")

	sm := &monitoringv1.ServiceMonitor{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "monitoring.coreos.com/v1",
			Kind:       "ServiceMonitor",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      backstage.Name + "-metrics",
			Namespace: backstage.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sm, func() error {
		sm.Labels = map[string]string{
			"app.kubernetes.io/instance": backstage.Name,
			"app.kubernetes.io/name":     "backstage",
		}

		sm.Spec = monitoringv1.ServiceMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/instance": backstage.Name,
					"app.kubernetes.io/name":     "backstage",
				},
			},
			NamespaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{backstage.Namespace},
			},
			Endpoints: []monitoringv1.Endpoint{
				{
					Port: "http-metrics",
					Path: "/metrics",
				},
			},
		}

		return controllerutil.SetControllerReference(backstage, sm, r.Scheme)
	})

	if err != nil {
		lg.Error(err, "Failed to create/update ServiceMonitor")
		return fmt.Errorf("failed to create/update ServiceMonitor: %w", err)
	}

	lg.Info("ServiceMonitor successfully created/updated", "name", sm.Name)
	return nil
}

// Helper to detect if ServiceMonitor CRD is installed
func (r *BackstageReconciler) serviceMonitorCRDExists(ctx context.Context) bool {
	crd := &apiextensionsv1.CustomResourceDefinition{}

	// Try to fetch the CRD for ServiceMonitor
	err := r.Client.Get(ctx, types.NamespacedName{
		Name: "servicemonitors.monitoring.coreos.com",
	}, crd)

	if err != nil {
		if apierrors.IsNotFound(err) {
			// CRD definitely not installed
			return false
		}
		// If it's another kind of API error, log but fail safe
		log.FromContext(ctx).Error(err, "Unable to check for ServiceMonitor CRD")
		return false
	}

	return true
}
