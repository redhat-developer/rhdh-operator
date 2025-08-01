package controller

import (
	"context"
	"fmt"
	"strings"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	bs "github.com/redhat-developer/rhdh-operator/api/v1alpha4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

	lg.Info("Monitoring enabled, creating/updating ServiceMonitor")

	sm := &monitoringv1.ServiceMonitor{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "monitoring.coreos.com/v1",
			Kind:       "ServiceMonitor",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      backstage.Name + "-metrics",
			Namespace: backstage.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/instance": backstage.Name,
				"app.kubernetes.io/name":     "backstage",
			},
		},
		Spec: monitoringv1.ServiceMonitorSpec{
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
		},
	}

	// Set controller reference
	if err := controllerutil.SetControllerReference(backstage, sm, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Use server-side apply for consistency with other resources
	if err := r.Patch(ctx, sm, client.Apply, &client.PatchOptions{
		FieldManager: BackstageFieldManager,
		Force:        ptr.To(true),
	}); err != nil {
		// Check if the error indicates that ServiceMonitor CRD is not installed
		if isServiceMonitorCRDNotFoundError(err) {
			lg.Error(err, "Monitoring enabled but ServiceMonitor CRD not found. Please install Prometheus Operator")
			return fmt.Errorf("monitoring enabled but ServiceMonitor CRD not found. Please install Prometheus Operator")
		}
		lg.Error(err, "Failed to apply ServiceMonitor")
		return fmt.Errorf("failed to apply ServiceMonitor: %w", err)
	}

	lg.Info("ServiceMonitor successfully applied", "name", sm.Name)
	return nil
}

// Helper to detect if the error indicates ServiceMonitor CRD is not found
func isServiceMonitorCRDNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	// Check for "no matches for kind" error which indicates CRD is not installed
	errMsg := err.Error()
	return strings.Contains(errMsg, "no matches for kind") && strings.Contains(errMsg, "ServiceMonitor") ||
		strings.Contains(errMsg, "the server could not find the requested resource") ||
		(apierrors.IsNotFound(err) && strings.Contains(errMsg, "servicemonitors.monitoring.coreos.com"))
}
