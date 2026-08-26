package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/redhat-developer/rhdh-operator/api"
	v1alpha5 "github.com/redhat-developer/rhdh-operator/api/v1alpha5"
	"github.com/redhat-developer/rhdh-operator/pkg/catalog"
)

const (
	// DefaultConfigMapName is the name of the default-config ConfigMap to patch
	// This should match the kustomize-generated name (namePrefix + base name)
	DefaultConfigMapName = "rhdh-default-config"
)

// DevHubPluginCatalogReconciler reconciles DevHubPluginCatalog objects
type DevHubPluginCatalogReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	OperatorNamespace string
	Processor         *catalog.Processor
}

// +kubebuilder:rbac:groups=rhdh.redhat.com,resources=devhubplugincatalogs,verbs=get;list;watch
// +kubebuilder:rbac:groups=rhdh.redhat.com,resources=devhubplugincatalogs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile handles any DevHubPluginCatalog change by reconciling ALL catalogs
func (r *DevHubPluginCatalogReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx)
	lg.V(1).Info("Reconciling DevHubPluginCatalog", "name", req.Name)

	// 1. List all catalogs
	catalogList := &api.DevHubPluginCatalogList{}
	if err := r.List(ctx, catalogList); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list catalogs: %w", err)
	}

	// 2. Build inputs from catalogs
	inputs, err := r.buildCatalogInputs(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 3. Set all catalogs to Processing (if any exist)
	if len(catalogList.Items) > 0 {
		r.setAllConditions(ctx, catalogList, metav1.ConditionFalse, v1alpha5.ConditionReasonProcessing, "Processing catalog")
	}

	// 4. Process catalogs (handles empty input → empty plugins list)
	content, err := r.Processor.Process(ctx, inputs)
	if err != nil {
		r.setAllConditions(ctx, catalogList, metav1.ConditionFalse, v1alpha5.ConditionReasonFailed, err.Error())
		return ctrl.Result{}, fmt.Errorf("failed to process catalogs: %w", err)
	}

	// 5. Apply merged ConfigMap (clears old config if no catalogs)
	if err := r.applyConfigMap(ctx, content); err != nil {
		r.setAllConditions(ctx, catalogList, metav1.ConditionFalse, v1alpha5.ConditionReasonFailed, err.Error())
		return ctrl.Result{}, fmt.Errorf("failed to apply ConfigMap: %w", err)
	}

	// 6. Set all catalogs to Ready (if any exist)
	if len(catalogList.Items) > 0 {
		r.setAllConditions(ctx, catalogList, metav1.ConditionTrue, v1alpha5.ConditionReasonSucceeded, "Catalog processed successfully")
	}

	lg.Info("Reconciled DevHubPluginCatalogs", "count", len(inputs))
	return ctrl.Result{}, nil
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
			if err := r.Get(ctx, key, secret); err != nil {
				return nil, fmt.Errorf("failed to get pull secret %q for catalog %q: %w", dhpc.Spec.Source.PullSecret.Name, dhpc.Name, err)
			}
			input.DockerConfig = secret.Data[".dockerconfigjson"]
		}

		// Get CA certificate
		if dhpc.Spec.Source.CertificateAuthority != nil {
			cm := &corev1.ConfigMap{}
			key := types.NamespacedName{Name: dhpc.Spec.Source.CertificateAuthority.Name, Namespace: r.OperatorNamespace}
			if err := r.Get(ctx, key, cm); err != nil {
				return nil, fmt.Errorf("failed to get CA ConfigMap %q for catalog %q: %w", dhpc.Spec.Source.CertificateAuthority.Name, dhpc.Name, err)
			}
			caKey := dhpc.Spec.Source.CertificateAuthority.Key
			if caKey == "" {
				caKey = "ca.crt"
			}
			input.CACert = []byte(cm.Data[caKey])
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

// setAllConditions sets the Ready condition on all catalogs in the list
func (r *DevHubPluginCatalogReconciler) setAllConditions(ctx context.Context, catalogList *api.DevHubPluginCatalogList, status metav1.ConditionStatus, reason, message string) {
	lg := log.FromContext(ctx)
	for i := range catalogList.Items {
		if err := r.setCondition(ctx, &catalogList.Items[i], status, reason, message); err != nil {
			lg.Error(err, "Failed to set condition", "catalog", catalogList.Items[i].Name)
		}
	}
}

// setCondition sets the Ready condition on a single catalog with retry on conflict
func (r *DevHubPluginCatalogReconciler) setCondition(ctx context.Context, dhpc *api.DevHubPluginCatalog, status metav1.ConditionStatus, reason, message string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Re-fetch the catalog to get the latest resourceVersion
		fresh := &api.DevHubPluginCatalog{}
		if err := r.Get(ctx, types.NamespacedName{Name: dhpc.Name, Namespace: dhpc.Namespace}, fresh); err != nil {
			return err
		}

		now := metav1.NewTime(time.Now())
		condition := metav1.Condition{
			Type:               v1alpha5.ConditionTypeReady,
			Status:             status,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: now,
		}

		// Update condition in status
		updated := false
		for i, c := range fresh.Status.Conditions {
			if c.Type == v1alpha5.ConditionTypeReady {
				if c.Status != status {
					fresh.Status.Conditions[i] = condition
				} else {
					// Only update message/reason, not LastTransitionTime
					fresh.Status.Conditions[i].Reason = reason
					fresh.Status.Conditions[i].Message = message
				}
				updated = true
				break
			}
		}
		if !updated {
			fresh.Status.Conditions = append(fresh.Status.Conditions, condition)
		}

		return r.Status().Update(ctx, fresh)
	})
}

// SetupWithManager sets up the controller with the Manager
func (r *DevHubPluginCatalogReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.DevHubPluginCatalog{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Named("devhubplugincatalog").
		Complete(r)
}
