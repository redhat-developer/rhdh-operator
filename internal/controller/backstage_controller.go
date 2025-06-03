package controller

import (
	"context"
	"fmt"
	"reflect"

	"github.com/redhat-developer/rhdh-operator/pkg/platform"

	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"k8s.io/utils/ptr"

	"k8s.io/apimachinery/pkg/types"

	openshift "github.com/openshift/api/route/v1"

	"k8s.io/apimachinery/pkg/api/meta"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/redhat-developer/rhdh-operator/pkg/model"

	bs "github.com/redhat-developer/rhdh-operator/api/v1alpha3"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	BackstageFieldManager = "backstage-controller"

	// AutoSyncEnvVar: EXT_CONF_SYNC_backstage env variable which defines the value for rhdh.redhat.com/ext-config-sync annotation of external config object (ConfigMap|Secret)
	// True by default
	AutoSyncEnvVar = "EXT_CONF_SYNC_backstage"

	// WatchExtConfig: WATCH_EXT_CONF_backstage if false disables watching external config objects (ConfigMaps|Secrets)
	// True by default
	WatchExtConfig = "WATCH_EXT_CONF_backstage"
)

// BackstageReconciler reconciles a Backstage object
type BackstageReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Platform platform.Platform
}

//+kubebuilder:rbac:groups=rhdh.redhat.com,resources=backstages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rhdh.redhat.com,resources=backstages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rhdh.redhat.com,resources=backstages/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=configmaps;secrets;services;persistentvolumeclaims,verbs=get;watch;create;update;list;delete;patch
//+kubebuilder:rbac:groups="",resources=persistentvolumes,verbs=get;list;watch
//+kubebuilder:rbac:groups="apps",resources=deployments;statefulsets,verbs=get;watch;create;update;list;delete;patch
//+kubebuilder:rbac:groups="route.openshift.io",resources=routes;routes/custom-host,verbs=get;watch;create;update;list;delete;patch
//+kubebuilder:rbac:groups="config.openshift.io",resources=ingresses,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *BackstageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx)

	backstage := bs.Backstage{}
	if err := r.Get(ctx, req.NamespacedName, &backstage); err != nil {
		if errors.IsNotFound(err) {
			lg.Info("backstage gone from the namespace")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to load backstage deployment from the cluster: %w", err)
	}

	// This update will make sure the status is always updated in case of any errors or successful result
	defer func(bs *bs.Backstage) {
		if err := r.Client.Status().Update(ctx, bs); err != nil {
			if errors.IsConflict(err) {
				lg.V(1).Info("Backstage object modified, retry syncing status", "Backstage Object", bs)
				return
			}
			lg.Error(err, "Error updating the Backstage resource status", "Backstage Object", bs)
		}
	}(&backstage)

	if len(backstage.Status.Conditions) == 0 {
		setStatusCondition(&backstage, bs.BackstageConditionTypeDeployed, metav1.ConditionFalse, bs.BackstageConditionReasonInProgress, "Deployment process started")
	}

	// 1. Preliminary read and prepare external config objects from the specs (configMaps, Secrets)
	// 2. Make some validation to fail fast
	externalConfig, err := r.preprocessSpec(ctx, backstage)
	if err != nil {
		return ctrl.Result{}, errorAndStatus(&backstage, "failed to preprocess backstage spec", err)
	}

	// This creates array of model objects to be reconsiled
	bsModel, err := model.InitObjects(ctx, backstage, externalConfig, r.Platform, r.Scheme)
	if err != nil {
		return ctrl.Result{}, errorAndStatus(&backstage, "failed to initialize backstage model", err)
	}

	// Apply the plugin dependencies
	if err := r.applyPluginDeps(ctx, req.NamespacedName, bsModel.DynamicPlugins); err != nil {
		return ctrl.Result{}, errorAndStatus(&backstage, "failed to apply plugin dependencies", err)
	}

	// Apply the runtime objects
	err = r.applyObjects(ctx, bsModel.RuntimeObjects)
	if err != nil {
		return ctrl.Result{}, errorAndStatus(&backstage, "failed to apply backstage objects", err)
	}

	if err := r.cleanObjects(ctx, backstage); err != nil {
		return ctrl.Result{}, errorAndStatus(&backstage, "failed to clean backstage objects ", err)
	}

	if false {
		r.unusedFn()
	}

	r.setDeploymentStatus(ctx, &backstage)
	return ctrl.Result{}, nil
}

func errorAndStatus(backstage *bs.Backstage, msg string, err error) error {
	setStatusCondition(backstage, bs.BackstageConditionTypeDeployed, metav1.ConditionFalse, bs.BackstageConditionReasonFailed, fmt.Sprintf("%s %s", msg, err))
	return fmt.Errorf("%s: %w", msg, err)
}

func (r *BackstageReconciler) unusedFn() {
	//FIXME(rm3l): remove this
	fmt.Println("[REMOVE ME] This should not show up in the logs!!!")
}

func (r *BackstageReconciler) applyObjects(ctx context.Context, objects []model.RuntimeObject) error {

	for _, obj := range objects {
		switch v := obj.Object().(type) {
		case client.Object:
			_, immutable := obj.(*model.DbSecret)
			if err := r.applyPayload(ctx, obj.Object().(client.Object), immutable); err != nil {
				return err
			}
		case *multiobject.MultiObject:
			mo := obj.Object().(*multiobject.MultiObject)
			for _, singleObject := range mo.Items {
				if err := r.applyPayload(ctx, singleObject, false); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unknown type %T! it should not happen normally", v)
		}
	}
	return nil
}

func objDispKind(obj client.Object, scheme *runtime.Scheme) string {
	gvk := utils.GetObjectKind(obj, scheme)
	if gvk == nil {
		return fmt.Sprintf("Unknown kind for: %s", reflect.TypeOf(obj).String())
	}
	return gvk.String()
}

func (r *BackstageReconciler) applyPayload(ctx context.Context, obj client.Object, immutable bool) error {
	lg := log.FromContext(ctx)
	if immutable {
		if err := r.Create(ctx, obj, &client.CreateOptions{FieldManager: BackstageFieldManager}); err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create object: %w", err)
			}
		} else {
			lg.V(1).Info("create object ", objDispKind(obj, r.Scheme), obj.GetName())
		}
		return nil
	}

	if err := r.Patch(ctx, obj, client.Apply, &client.PatchOptions{FieldManager: BackstageFieldManager, Force: ptr.To(true)}); err != nil {
		return r.tryToFixUpgradeGlitch(ctx, obj, err)
	}
	lg.V(1).Info("apply object ", objDispKind(obj, r.Scheme), obj.GetName())
	return nil
}

func (r *BackstageReconciler) cleanObjects(ctx context.Context, backstage bs.Backstage) error {

	const failedToCleanup = "failed to cleanup runtime"
	// check if local database disabled, respective objects have to deleted/unowned
	if !backstage.Spec.IsLocalDbEnabled() {
		if err := r.tryToDelete(ctx, &appsv1.StatefulSet{}, model.DbStatefulSetName(backstage.Name), backstage.Namespace); err != nil {
			return fmt.Errorf("%s %w", failedToCleanup, err)
		}
		if err := r.tryToDelete(ctx, &corev1.Service{}, model.DbServiceName(backstage.Name), backstage.Namespace); err != nil {
			return fmt.Errorf("%s %w", failedToCleanup, err)
		}
		if err := r.tryToDelete(ctx, &corev1.Secret{}, model.DbSecretDefaultName(backstage.Name), backstage.Namespace); err != nil {
			return fmt.Errorf("%s %w", failedToCleanup, err)
		}
	}

	//// check if route disabled, respective objects have to deleted/unowned
	if r.Platform.IsOpenshift() && !backstage.Spec.IsRouteEnabled() {
		if err := r.tryToDelete(ctx, &openshift.Route{}, model.RouteName(backstage.Name), backstage.Namespace); err != nil {
			return fmt.Errorf("%s %w", failedToCleanup, err)
		}
	}

	return nil
}

// tryToDelete tries to delete the object by name and namespace, does not throw error if object not found
func (r *BackstageReconciler) tryToDelete(ctx context.Context, obj client.Object, name string, ns string) error {
	obj.SetName(name)
	obj.SetNamespace(ns)
	if err := r.Delete(ctx, obj); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete %s: %w", name, err)
	}
	return nil
}

func (r *BackstageReconciler) setDeploymentStatus(ctx context.Context, backstage *bs.Backstage) {
	deploy := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: model.DeploymentName(backstage.Name), Namespace: backstage.GetNamespace()}, deploy); err != nil {
		setStatusCondition(backstage, bs.BackstageConditionTypeDeployed, metav1.ConditionFalse, bs.BackstageConditionReasonFailed, err.Error())
		return
	}

	if deploy.Status.ReadyReplicas == deploy.Status.Replicas {
		setStatusCondition(backstage, bs.BackstageConditionTypeDeployed, metav1.ConditionTrue, bs.BackstageConditionReasonDeployed, "")
	} else {
		msg := "Deployment status:"
		for _, c := range deploy.Status.Conditions {
			if c.Type == appsv1.DeploymentAvailable {
				msg += " Available: " + c.Message
			} else if c.Type == appsv1.DeploymentProgressing {
				msg += " Progressing: " + c.Message
			}
		}
		setStatusCondition(backstage, bs.BackstageConditionTypeDeployed, metav1.ConditionFalse, bs.BackstageConditionReasonInProgress, msg)
	}
}

func setStatusCondition(backstage *bs.Backstage, condType bs.BackstageConditionType, status metav1.ConditionStatus, reason bs.BackstageConditionReason, msg string) {
	meta.SetStatusCondition(&backstage.Status.Conditions, metav1.Condition{
		Type:               string(condType),
		Status:             status,
		LastTransitionTime: metav1.Time{},
		Reason:             string(reason),
		Message:            msg,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackstageReconciler) SetupWithManager(mgr ctrl.Manager) error {

	b := ctrl.NewControllerManagedBy(mgr).
		For(&bs.Backstage{})

	if err := r.addWatchers(b); err != nil {
		return err
	}

	return b.Complete(r)
}

func (r *BackstageReconciler) tryToFixUpgradeGlitch(ctx context.Context, obj client.Object, inError error) error {

	lg := log.FromContext(ctx)

	lg.V(1).Info(
		"failed to apply object => trying to delete it (and losing any custom labels/annotations on it) so it can be recreated upon next reconciliation...",
		objDispKind(obj, r.Scheme), obj.GetName(),
		"cause", inError,
	)
	// Some resources like StatefulSets allow patching a limited set of fields. A FieldValueForbidden error is returned.
	// Some other resources like Services do not support updating the primary/secondary clusterIP || ipFamily. A FieldValueInvalid error is returned.
	// That's why we are trying to delete them first, taking care of orphaning the dependents so that they can be retained.
	// They will be recreated at the next reconciliation.
	// If they cannot be recreated at the next reconciliation, the expected error will be returned.
	if err := r.Delete(ctx, obj, client.PropagationPolicy(metav1.DeletePropagationOrphan)); err != nil {
		return fmt.Errorf("failed to delete object %s so it can be recreated: %w", obj, err)
	}
	lg.V(1).Info("deleted object. If you had set any custom labels/annotations on it manually, you will need to add them again",
		objDispKind(obj, r.Scheme), obj.GetName(),
	)
	return nil
}
