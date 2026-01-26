package controller

import (
	"context"
	"fmt"

	bs "github.com/redhat-developer/rhdh-operator/api/v1alpha5"
	"github.com/redhat-developer/rhdh-operator/pkg/model"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// applyImageStreams creates ImageStream resources on OpenShift clusters.
// ImageStreams allow the internal registry to mirror images from external registries
// using the cluster's pull secret, which the init container can then access.
func (r *BackstageReconciler) applyImageStreams(ctx context.Context, backstage bs.Backstage) error {
	lg := log.FromContext(ctx)

	// Only apply ImageStreams on OpenShift
	if !r.Platform.IsOpenshift() {
		lg.V(1).Info("Not running on OpenShift, skipping ImageStream creation")
		return nil
	}

	objects, err := model.GetImageStreams(backstage, r.Scheme, r.Platform.IsOpenshift())
	if err != nil {
		return fmt.Errorf("failed to get imagestreams: %w", err)
	}

	if len(objects) == 0 {
		lg.V(1).Info("No ImageStream manifests found")
		return nil
	}

	lg.Info("Applying ImageStreams for registry.redhat.io image access", "count", len(objects))

	var errs []error
	for _, obj := range objects {
		lg.V(1).Info("Applying ImageStream", "name", obj.GetName(), "namespace", obj.GetNamespace())

		if err = r.Patch(ctx, obj, client.Apply, &client.PatchOptions{
			FieldManager: BackstageFieldManager,
			Force:        ptr.To(true),
		}); err != nil {
			lg.Error(err, "Failed to apply ImageStream", "name", obj.GetName())
			errs = append(errs, fmt.Errorf("failed to apply imagestream %s: %w", obj.GetName(), err))
		}
	}

	if len(errs) > 0 {
		return combineErrors(errs)
	}

	return nil
}
