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

	// 1. List catalogs in operator namespace only
	catalogList := &api.DevHubPluginCatalogList{}
	if err := r.List(ctx, catalogList, client.InNamespace(r.OperatorNamespace)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list catalogs: %w", err)
	}

	// 2. Process each catalog
	plugins := make(catalog.PluginMap)
	for i := range catalogList.Items {
		dhpc := &catalogList.Items[i]
		r.setCondition(ctx, dhpc, metav1.ConditionFalse, v1alpha5.ConditionReasonProcessing, "Processing catalog")

		input, err := r.buildCatalogInput(ctx, dhpc)
		if err != nil {
			r.setCondition(ctx, dhpc, metav1.ConditionFalse, v1alpha5.ConditionReasonFailed, err.Error())
			return ctrl.Result{}, err
		}
		if err := r.Processor.AddCatalog(ctx, input, plugins); err != nil {
			r.setCondition(ctx, dhpc, metav1.ConditionFalse, v1alpha5.ConditionReasonFailed, err.Error())
			return ctrl.Result{}, fmt.Errorf("failed to process catalog %q: %w", dhpc.Name, err)
		}
	}

	// 3. Apply merged ConfigMap (clears old config if no catalogs)
	if err := r.applyConfigMap(ctx, plugins); err != nil {
		r.setAllConditions(ctx, catalogList, metav1.ConditionFalse, v1alpha5.ConditionReasonFailed, err.Error())
		return ctrl.Result{}, fmt.Errorf("failed to apply ConfigMap: %w", err)
	}

	// 4. Set all catalogs to Ready
	r.setAllConditions(ctx, catalogList, metav1.ConditionTrue, v1alpha5.ConditionReasonSucceeded, "Catalog processed successfully")

	lg.Info("Reconciled DevHubPluginCatalogs", "count", len(catalogList.Items))
	return ctrl.Result{}, nil
}

// buildCatalogInput builds a processor input from a single catalog
func (r *DevHubPluginCatalogReconciler) buildCatalogInput(ctx context.Context, dhpc *api.DevHubPluginCatalog) (catalog.CatalogInput, error) {
	input := catalog.CatalogInput{
		Ref:           dhpc.Spec.Source.Ref,
		SkipTLSVerify: dhpc.Spec.Source.SkipTLSVerify,
	}

	// Get pull secret
	if dhpc.Spec.Source.PullSecret != nil {
		secret := &corev1.Secret{}
		key := types.NamespacedName{Name: dhpc.Spec.Source.PullSecret.Name, Namespace: r.OperatorNamespace}
		if err := r.Get(ctx, key, secret); err != nil {
			return input, fmt.Errorf("failed to get pull secret %q for catalog %q: %w", dhpc.Spec.Source.PullSecret.Name, dhpc.Name, err)
		}
		input.DockerConfig = secret.Data[".dockerconfigjson"]
	}

	// Get CA certificate
	if dhpc.Spec.Source.CertificateAuthority != nil {
		cm := &corev1.ConfigMap{}
		key := types.NamespacedName{Name: dhpc.Spec.Source.CertificateAuthority.Name, Namespace: r.OperatorNamespace}
		if err := r.Get(ctx, key, cm); err != nil {
			return input, fmt.Errorf("failed to get CA ConfigMap %q for catalog %q: %w", dhpc.Spec.Source.CertificateAuthority.Name, dhpc.Name, err)
		}
		caKey := dhpc.Spec.Source.CertificateAuthority.Key
		if caKey == "" {
			caKey = "ca.crt"
		}
		input.CACert = []byte(cm.Data[caKey])
	}

	return input, nil
}

// applyConfigMap builds patch from plugins and applies it to the default-config ConfigMap.
func (r *DevHubPluginCatalogReconciler) applyConfigMap(ctx context.Context, plugins catalog.PluginMap) error {
	patchBytes, err := r.Processor.BuildPatch(plugins)
	if err != nil {
		return fmt.Errorf("failed to build patch: %w", err)
	}

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
	for i := range catalogList.Items {
		r.setCondition(ctx, &catalogList.Items[i], status, reason, message)
	}
}

// setCondition sets the Ready condition on a single catalog with retry on conflict.
// Errors are logged internally; controller will retry on next reconciliation.
func (r *DevHubPluginCatalogReconciler) setCondition(ctx context.Context, dhpc *api.DevHubPluginCatalog, status metav1.ConditionStatus, reason, message string) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
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
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to set condition", "catalog", dhpc.Name, "reason", reason)
	}
}

// SetupWithManager sets up the controller with the Manager
func (r *DevHubPluginCatalogReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.DevHubPluginCatalog{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Named("devhubplugincatalog").
		Complete(r)
}
