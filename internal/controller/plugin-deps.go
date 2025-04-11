package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *BackstageReconciler) applyPluginDeps(ctx context.Context, bsNamespace string) error {

	lg := log.FromContext(ctx)
	dir := filepath.Join(os.Getenv("LOCALBIN"), "default-config", "plugin-deps")

	// Read all YAML files from the directory
	//objects, err := utils.ReadYamlFilesFromDir(dir)
	objects, err := utils.ReadPluginDeps(dir)

	if err != nil {
		return fmt.Errorf("failed to read YAML files: %w", err)
	}

	// Process the objects as needed
	var errs []error
	for _, obj := range objects {
		// Apply the unstructured object
		lg.V(1).Info("apply plugin dependency: ", "name", obj.GetName(), "kind", obj.GetKind(), "namespace", obj.GetNamespace())

		// Set the namespace if not set
		if obj.GetNamespace() == "" {
			obj.SetNamespace(bsNamespace)
		}

		if err := r.Patch(ctx, obj, client.Apply, &client.PatchOptions{FieldManager: BackstageFieldManager, Force: ptr.To(true)}); err != nil {
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
