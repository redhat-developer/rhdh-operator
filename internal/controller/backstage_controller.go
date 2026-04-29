package controller

import (
	"context"
	"fmt"
	"reflect"

	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/platform"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"k8s.io/utils/ptr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/redhat-developer/rhdh-operator/pkg/model"

	"github.com/redhat-developer/rhdh-operator/api"

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
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *BackstageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx)

	backstage := api.Backstage{}
	if err := r.Get(ctx, req.NamespacedName, &backstage); err != nil {
		if errors.IsNotFound(err) {
			lg.Info("backstage gone from the namespace")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to load backstage deployment from the cluster: %w", err)
	}

	// This update will make sure the status is always updated in case of any errors or successful result
	defer func(bs *api.Backstage) {
		if err := r.Client.Status().Update(ctx, bs); err != nil {
			if errors.IsConflict(err) {
				lg.V(1).Info("Backstage object modified, retry syncing status", "Backstage Object", bs)
				return
			}
			lg.Error(err, "Error updating the Backstage resource status", "Backstage Object", bs)
		}
	}(&backstage)

	if len(backstage.Status.Conditions) == 0 {
		setStatusCondition(&backstage, api.BackstageConditionTypeDeployed, metav1.ConditionFalse, api.BackstageConditionReasonInProgress, "Deployment process started")
	}

	// 1. Preliminary read and prepare external config objects from the specs (configMaps, Secrets)
	// 2. Make some validation to fail fast
	externalConfig, err := r.preprocessSpec(ctx, backstage)
	if err != nil {
		return ctrl.Result{}, errorAndStatus(&backstage, "failed to preprocess backstage spec", err)
	}

	// Apply the ServiceMonitor if monitoring is enabled
	if err := r.applyServiceMonitor(ctx, &backstage); err != nil {
		return ctrl.Result{}, errorAndStatus(&backstage, "failed to apply ServiceMonitor", err)
	}

	// This creates array of model objects to be reconsiled
	bsModel, err := model.InitObjects(ctx, backstage, externalConfig, r.Platform, r.Scheme)
	if err != nil {
		return ctrl.Result{}, errorAndStatus(&backstage, "failed to initialize backstage model", err)
	}

	// Apply the plugin dependencies
	if err := r.applyPluginDeps(ctx, backstage, bsModel.DynamicPlugins); err != nil {
		return ctrl.Result{}, errorAndStatus(&backstage, "failed to apply plugin dependencies", err)
	}

	// Apply the runtime objects
	err = r.applyObjects(ctx, bsModel.RuntimeObjects)
	if err != nil {
		return ctrl.Result{}, errorAndStatus(&backstage, "failed to apply backstage objects", err)
	}

	r.setDeploymentStatus(ctx, &backstage, *bsModel)
	return ctrl.Result{}, nil
}

func errorAndStatus(backstage *api.Backstage, msg string, err error) error {
	setStatusCondition(backstage, api.BackstageConditionTypeDeployed, metav1.ConditionFalse, api.BackstageConditionReasonFailed, fmt.Sprintf("%s %s", msg, err))
	return fmt.Errorf("%s: %w", msg, err)
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
		return fmt.Errorf("failed to apply object: %w", err)
	}
	lg.V(1).Info("apply object ", objDispKind(obj, r.Scheme), obj.GetName())
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackstageReconciler) SetupWithManager(mgr ctrl.Manager) error {

	b := ctrl.NewControllerManagedBy(mgr).
		For(&api.Backstage{})

	if err := r.addWatchers(b); err != nil {
		return err
	}

	return b.Complete(r)
}
