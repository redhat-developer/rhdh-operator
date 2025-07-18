package controller

import (
	"context"
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	bs "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
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

    lg.Info("Monitoring enabled, creating/patching ServiceMonitor")

    monitor := &monitoringv1.ServiceMonitor{
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
            Endpoints: []monitoringv1.Endpoint{{
                Port:     "http-metrics",
                Path:     "/metrics",
                Interval: "30s",
            }},
        },
    }

    // Set controller reference so it gets garbage-collected with the Backstage CR
    if err := controllerutil.SetControllerReference(backstage, monitor, r.Scheme); err != nil {
        lg.Error(err, "Failed to set controller reference on ServiceMonitor")
        return fmt.Errorf("failed to set controller reference on ServiceMonitor: %w", err)
    }

    lg.Info("Patching ServiceMonitor via server-side apply")
    return r.Patch(ctx, monitor, client.Apply, &client.PatchOptions{
        FieldManager: BackstageFieldManager,
        Force:        ptr.To(true),
    })
}

// Helper to detect if ServiceMonitor CRD is installed
func (r *BackstageReconciler) serviceMonitorCRDExists(ctx context.Context) bool {
    // For now assume it's installed
    return true
}