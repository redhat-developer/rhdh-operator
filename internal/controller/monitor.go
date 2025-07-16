package controller

import (
	"context"
	"fmt"

	bs "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *BackstageReconciler) applyServiceMonitor(ctx context.Context, backstage *bs.Backstage) error {
    if backstage.Spec.Monitoring == nil || !backstage.Spec.Monitoring.Enabled {
        // If monitoring is disabled, delete any existing ServiceMonitor
        return r.tryToDelete(ctx,
            &monitoringv1.ServiceMonitor{},
            backstage.Name+"-metrics",
            backstage.Namespace,
        )
    }

    // Check if ServiceMonitor CRD exists before creating
    if !r.serviceMonitorCRDExists(ctx) {
        return fmt.Errorf("monitoring enabled but ServiceMonitor CRD not found. Please install Prometheus Operator")
    }

    monitor := &monitoringv1.ServiceMonitor{
        ObjectMeta: metav1.ObjectMeta{
            Name:      backstage.Name + "-metrics",
            Namespace: backstage.Namespace,
            Labels: map[string]string{
                "app.kubernetes.io/name": backstage.Name,
            },
        },
        Spec: monitoringv1.ServiceMonitorSpec{
            Selector: metav1.LabelSelector{
                MatchLabels: map[string]string{
                    "app.kubernetes.io/name": backstage.Name,
                },
            },
            Endpoints: []monitoringv1.Endpoint{{
                Port: "metrics",
            }},
        },
    }

	// Set controller reference so it gets garbage-collected with the Backstage CR
	if err := controllerutil.SetControllerReference(backstage, monitor, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on ServiceMonitor: %w", err)
	}

	// Use server-side apply
    return r.Patch(ctx, monitor, client.Apply, &client.PatchOptions{
        FieldManager: BackstageFieldManager,
        Force:        ptr.To(true),
    })
}

// Helper to detect if ServiceMonitor CRD is installed
func (r *BackstageReconciler) serviceMonitorCRDExists(ctx context.Context) bool {
    // ServiceMonitor belongs to group monitoring.coreos.com/v1
    gk := monitoringv1.SchemeGroupVersion.WithKind("ServiceMonitor").GroupKind()

    // Ask the RESTMapper for a mapping (this will fail if CRD not installed)
    _, err := r.Client.RESTMapper().RESTMapping(gk, monitoringv1.SchemeGroupVersion.Version)
    return err == nil
}
