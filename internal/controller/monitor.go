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
	// Don't proceed if monitoring is nil or not enabled
	if backstage.Spec.Monitoring == nil || !backstage.Spec.Monitoring.Enabled {
		return nil
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
