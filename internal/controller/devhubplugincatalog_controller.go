package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/catalog"
)

const (
	// DefaultConfigMapName is the name of the default-config ConfigMap to patch
	// This should match the kustomize-generated name (namePrefix + base name)
	DefaultConfigMapName = "rhdh-default-config"

	catalogRequeueOnError   = 1 * time.Minute
	catalogRequeueOnSuccess = 5 * time.Minute
)

// DevHubPluginCatalogReconciler reconciles DevHubPluginCatalog objects
type DevHubPluginCatalogReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	OperatorNamespace string
	Processor         *catalog.Processor
}

// +kubebuilder:rbac:groups=rhdh.redhat.com,resources=devhubplugincatalogs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile handles any DevHubPluginCatalog change by reconciling ALL catalogs
func (r *DevHubPluginCatalogReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx)
	lg.V(1).Info("Reconciling DevHubPluginCatalog", "name", req.Name)

	// 1. List all catalogs and build inputs
	inputs, err := r.buildCatalogInputs(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	if len(inputs) == 0 {
		lg.Info("No DevHubPluginCatalog resources found")
		return ctrl.Result{}, nil
	}

	// 2. Process all catalogs (fetch and merge)
	content, err := r.Processor.Process(ctx, inputs)
	if err != nil {
		return ctrl.Result{RequeueAfter: catalogRequeueOnError}, fmt.Errorf("failed to process catalogs: %w", err)
	}

	// 3. Apply merged ConfigMap
	if err := r.applyConfigMap(ctx, content); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to apply ConfigMap: %w", err)
	}

	lg.Info("Successfully reconciled all DevHubPluginCatalogs", "count", len(inputs))
	return ctrl.Result{RequeueAfter: catalogRequeueOnSuccess}, nil
}

// buildCatalogInputs lists all catalogs and builds processor inputs
func (r *DevHubPluginCatalogReconciler) buildCatalogInputs(ctx context.Context) ([]catalog.CatalogInput, error) {
	catalogList := &api.DevHubPluginCatalogList{}
	if err := r.List(ctx, catalogList); err != nil {
		return nil, fmt.Errorf("failed to list catalogs: %w", err)
	}

	inputs := make([]catalog.CatalogInput, 0, len(catalogList.Items))

	for i := range catalogList.Items {
		dhpc := &catalogList.Items[i]

		input := catalog.CatalogInput{
			Ref:           dhpc.Spec.Source.Ref,
			SkipTLSVerify: dhpc.Spec.Source.SkipTLSVerify,
		}

		// Get pull secret
		if dhpc.Spec.Source.PullSecret != nil {
			secret := &corev1.Secret{}
			key := types.NamespacedName{Name: dhpc.Spec.Source.PullSecret.Name, Namespace: r.OperatorNamespace}
			if err := r.Get(ctx, key, secret); err == nil {
				input.DockerConfig = secret.Data[".dockerconfigjson"]
			}
		}

		// Get CA certificate
		if dhpc.Spec.Source.CertificateAuthority != nil {
			cm := &corev1.ConfigMap{}
			key := types.NamespacedName{Name: dhpc.Spec.Source.CertificateAuthority.Name, Namespace: r.OperatorNamespace}
			if err := r.Get(ctx, key, cm); err == nil {
				caKey := dhpc.Spec.Source.CertificateAuthority.Key
				if caKey == "" {
					caKey = "ca.crt"
				}
				input.CACert = []byte(cm.Data[caKey])
			}
		}

		inputs = append(inputs, input)
	}

	return inputs, nil
}

// applyConfigMap patches the default-config ConfigMap with pre-built patch bytes.
func (r *DevHubPluginCatalogReconciler) applyConfigMap(ctx context.Context, patchBytes []byte) error {
	cm := &corev1.ConfigMap{}
	cm.Name = DefaultConfigMapName
	cm.Namespace = r.OperatorNamespace

	if err := r.Patch(ctx, cm, client.RawPatch(types.MergePatchType, patchBytes)); err != nil {
		return fmt.Errorf("failed to patch ConfigMap %s: %w", DefaultConfigMapName, err)
	}

	log.FromContext(ctx).V(1).Info("Patched ConfigMap with catalog content", "name", cm.Name)
	return nil
}

// SetupWithManager sets up the controller with the Manager
func (r *DevHubPluginCatalogReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.DevHubPluginCatalog{}).
		Named("devhubplugincatalog").
		Complete(r)
}
