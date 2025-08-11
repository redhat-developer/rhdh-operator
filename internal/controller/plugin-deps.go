package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/redhat-developer/rhdh-operator/pkg/model"

	bs "github.com/redhat-developer/rhdh-operator/api/v1alpha4"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *BackstageReconciler) applyPluginDeps(ctx context.Context, backstage bs.Backstage, plugins model.DynamicPlugins) error {

	lg := log.FromContext(ctx)

	objects, err := model.GetPluginDeps(backstage, plugins, r.Scheme)
	if err != nil {
		return fmt.Errorf("failed to get plugin dependencies: %w", err)
	}

	// Process the objects as needed
	var errs []error
	for _, obj := range objects {
		// Apply the unstructured object
		lg.V(1).Info("apply plugin dependency: ", "name", obj.GetName(), "kind", obj.GetKind(), "namespace", obj.GetNamespace(), "for plugins: ", plugins)

		if err = r.Patch(ctx, obj, client.Apply, &client.PatchOptions{FieldManager: BackstageFieldManager, Force: ptr.To(true)}); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return combineErrors(errs)
	}

	return nil
}

func combineErrors(errs []error) error {
	var sb strings.Builder
	for _, err := range errs {
		sb.WriteString(err.Error() + "\n")
	}
	return errors.New(sb.String())
}
