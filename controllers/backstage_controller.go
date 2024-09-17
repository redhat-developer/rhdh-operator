//
// Copyright (c) 2023 Red Hat, Inc.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/types"

	openshift "github.com/openshift/api/route/v1"

	"k8s.io/apimachinery/pkg/api/meta"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"

	appsv1 "k8s.io/api/apps/v1"

	"redhat-developer/red-hat-developer-hub-operator/pkg/model"

	bs "redhat-developer/red-hat-developer-hub-operator/api/v1alpha2"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	BackstageFieldManager = "backstage-controller"
)

var watchedConfigSelector = metav1.LabelSelector{
	MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      model.ExtConfigSyncLabel,
			Values:   []string{"true"},
			Operator: metav1.LabelSelectorOpIn,
		},
	},
}

// BackstageReconciler reconciles a Backstage object
type BackstageReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// If true, Backstage Controller always sync the state of runtime objects created
	// otherwise, runtime objects can be re-configured independently
	OwnsRuntime bool
	// indicates if current cluster is Openshift
	IsOpenShift bool
}

//+kubebuilder:rbac:groups=rhdh.redhat.com,resources=backstages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rhdh.redhat.com,resources=backstages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rhdh.redhat.com,resources=backstages/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=configmaps;secrets;services,verbs=get;watch;create;update;list;delete;patch
//+kubebuilder:rbac:groups="",resources=persistentvolumes;persistentvolumeclaims,verbs=get;list;watch
//+kubebuilder:rbac:groups="apps",resources=deployments;statefulsets,verbs=get;watch;create;update;list;delete;patch
//+kubebuilder:rbac:groups="route.openshift.io",resources=routes;routes/custom-host,verbs=get;watch;create;update;list;delete;patch

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
	bsModel, err := model.InitObjects(ctx, backstage, externalConfig, r.OwnsRuntime, r.IsOpenShift, r.Scheme)
	if err != nil {
		return ctrl.Result{}, errorAndStatus(&backstage, "failed to initialize backstage model", err)
	}

	err = r.applyObjects(ctx, bsModel.RuntimeObjects)
	if err != nil {
		return ctrl.Result{}, errorAndStatus(&backstage, "failed to apply backstage objects", err)
	}

	if err := r.cleanObjects(ctx, backstage); err != nil {
		return ctrl.Result{}, errorAndStatus(&backstage, "failed to clean backstage objects ", err)
	}

	setStatusCondition(&backstage, bs.BackstageConditionTypeDeployed, metav1.ConditionTrue, bs.BackstageConditionReasonDeployed, "")

	return ctrl.Result{}, nil
}

func errorAndStatus(backstage *bs.Backstage, msg string, err error) error {
	setStatusCondition(backstage, bs.BackstageConditionTypeDeployed, metav1.ConditionFalse, bs.BackstageConditionReasonFailed, fmt.Sprintf("%s %s", msg, err))
	return fmt.Errorf("%s %w", msg, err)
}

func (r *BackstageReconciler) applyObjects(ctx context.Context, objects []model.RuntimeObject) error {

	lg := log.FromContext(ctx)

	for _, obj := range objects {

		//baseObject := obj.EmptyObject()
		// do not read Secrets
		if _, ok := obj.Object().(*corev1.Secret); ok {
			// try to create. patch will not work here as it does not throw errors.IsAlreadyExists
			if err := r.Create(ctx, obj.Object(), &client.CreateOptions{FieldManager: BackstageFieldManager}); err != nil {
				//r.Patch(ctx, obj.Object(), client.Apply, &client.PatchOptions{BackstageFieldManager: BackstageFieldManager});

				if !errors.IsAlreadyExists(err) {
					return fmt.Errorf("failed to create secret: %w", err)
				}
				//if DBSecret - nothing to do, it is not for update
				if _, ok := obj.(*model.DbSecret); ok {
					continue
				}
			} else {
				lg.V(1).Info("create secret ", objDispName(obj), obj.Object().GetName())
				continue
			}

		}
		//else {
		//if err := r.Patch(ctx, obj.Object(), client.Apply, &client.PatchOptions{FieldManager: BackstageFieldManager, Force: ptr.To(true)}); err != nil {
		//	return fmt.Errorf("failed to patch object %s: %w", objDispName(obj), err)
		//}
		//lg.V(1).Info("patch object ", objDispName(obj), obj.Object().GetName())

		//if err := r.Get(ctx, types.NamespacedName{Name: obj.Object().GetName(), Namespace: obj.Object().GetNamespace()}, baseObject); err != nil {
		//
		//
		//	if !errors.IsNotFound(err) {
		//		return fmt.Errorf("failed to get object: %w", err)
		//	}
		//
		//	if err := r.Patch(ctx, obj.Object(), client.Apply, &client.PatchOptions{FieldManager: BackstageFieldManager, Force: ptr.To(true)}); err != nil {
		//		return fmt.Errorf("failed to create object %w", err)
		//	}
		//
		//	lg.V(1).Info("create object ", objDispName(obj), obj.Object().GetName())
		//	continue
		//}
		//}

		if err := r.Patch(ctx, obj.Object(), client.Apply, &client.PatchOptions{FieldManager: BackstageFieldManager, Force: ptr.To(true)}); err != nil {
			return fmt.Errorf("failed to patch object %s: %w", objDispName(obj), err)
		}
		lg.V(1).Info("patch object ", objDispName(obj), obj.Object().GetName())

		//if err := r.patchObject(ctx, baseObject, obj); err != nil {
		//	lg.V(1).Info(
		//		"failed to patch object => trying to delete it (and losing any custom labels/annotations on it) so it can be recreated upon next reconciliation...",
		//		objDispName(obj), obj.Object().GetName(),
		//		"cause", err,
		//	)
		//	// Some resources like StatefulSets allow patching a limited set of fields. A FieldValueForbidden error is returned.
		//	// Some other resources like Services do not support updating the primary/secondary clusterIP || ipFamily. A FieldValueInvalid error is returned.
		//	// That's why we are trying to delete them first, taking care of orphaning the dependents so that they can be retained.
		//	// They will be recreated at the next reconciliation.
		//	// If they cannot be recreated at the next reconciliation, the expected error will be returned.
		//	if err = r.Delete(ctx, baseObject, client.PropagationPolicy(metav1.DeletePropagationOrphan)); err != nil {
		//		return fmt.Errorf("failed to delete object %s so it can be recreated: %w", obj.Object(), err)
		//	}
		//	lg.V(1).Info("deleted object. If you had set any custom labels/annotations on it manually, you will need to add them again",
		//		objDispName(obj), obj.Object().GetName(),
		//	)
		//} else {
		//	lg.V(1).Info("patch object ", objDispName(obj), obj.Object().GetName())
		//}
	}
	return nil
}

func objDispName(obj model.RuntimeObject) string {
	return reflect.TypeOf(obj.Object()).String()
}

//func (r *BackstageReconciler) patchObject(ctx context.Context, baseObject client.Object, obj model.RuntimeObject) error {
//
//	//lg := log.FromContext(ctx)
//
//	// restore labels and annotations
//	if baseObject.GetLabels() != nil {
//		if obj.Object().GetLabels() == nil {
//			obj.Object().SetLabels(map[string]string{})
//		}
//		for name, value := range baseObject.GetLabels() {
//			if obj.Object().GetLabels()[name] == "" {
//				obj.Object().GetLabels()[name] = value
//			}
//		}
//	}
//	if baseObject.GetAnnotations() != nil {
//		if obj.Object().GetAnnotations() == nil {
//			obj.Object().SetAnnotations(map[string]string{})
//		}
//		for name, value := range baseObject.GetAnnotations() {
//			if obj.Object().GetAnnotations()[name] == "" {
//				obj.Object().GetAnnotations()[name] = value
//			}
//		}
//	}
//
//	// needed for openshift.Route only, Openshift yells otherwise
//	//obj.Object().SetResourceVersion(baseObject.GetResourceVersion())
//	//if objectKind, ok := obj.Object().(schema.ObjectKind); ok {
//	//	objectKind.SetGroupVersionKind(baseObject.GetObjectKind().GroupVersionKind())
//	//}
//
//	if err := r.Patch(ctx, obj.Object(), client.Apply, &client.PatchOptions{FieldManager: BackstageFieldManager, Force: ptr.To(true)}); err != nil {
//		return fmt.Errorf("failed to patch object %s: %w", objDispName(obj), err)
//	}
//
//	return nil
//}

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
	if r.IsOpenShift && !backstage.Spec.IsRouteEnabled() {
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

func setStatusCondition(backstage *bs.Backstage, condType bs.BackstageConditionType, status metav1.ConditionStatus, reason bs.BackstageConditionReason, msg string) {
	meta.SetStatusCondition(&backstage.Status.Conditions, metav1.Condition{
		Type:               string(condType),
		Status:             status,
		LastTransitionTime: metav1.Time{},
		Reason:             string(reason),
		Message:            msg,
	})
}

// requestByLabel returns a request with current Namespace and Backstage Object name taken from label
// or empty request object if label not found
func (r *BackstageReconciler) requestByLabel(ctx context.Context, object client.Object) []reconcile.Request {

	lg := log.FromContext(ctx)

	backstageName := object.GetAnnotations()[model.BackstageNameAnnotation]
	if backstageName == "" {
		//lg.V(1).Info(fmt.Sprintf("warning: %s annotation is not defined for %s, Backstage instances will not be reconciled in this loop", model.BackstageNameAnnotation, object.GetName()))
		return []reconcile.Request{}
	}

	nn := types.NamespacedName{
		Namespace: object.GetNamespace(),
		Name:      backstageName,
	}

	backstage := bs.Backstage{}
	if err := r.Get(ctx, nn, &backstage); err != nil {
		if !errors.IsNotFound(err) {
			lg.Error(err, "request by label failed, get Backstage ")
		}
		return []reconcile.Request{}
	}

	ec, err := r.preprocessSpec(ctx, backstage)
	if err != nil {
		lg.Error(err, "request by label failed, preprocess Backstage ")
		return []reconcile.Request{}
	}

	deploy := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: model.DeploymentName(backstage.Name), Namespace: object.GetNamespace()}, deploy); err != nil {
		if errors.IsNotFound(err) {
			lg.V(1).Info("request by label, deployment not found", "name", model.DeploymentName(backstage.Name))
		} else {
			lg.Error(err, "request by label failed, get Deployment ", "error ", err)
		}
		return []reconcile.Request{}
	}

	newHash := ec.GetHash()
	oldHash := deploy.Spec.Template.ObjectMeta.GetAnnotations()[model.ExtConfigHashAnnotation]
	if newHash == oldHash {
		lg.V(1).Info("request by label, hash are equal", "hash", newHash)
		return []reconcile.Request{}
	}

	lg.V(1).Info("enqueuing reconcile for", object.GetObjectKind().GroupVersionKind().Kind, object.GetName(), "new hash: ", newHash, "old hash: ", oldHash)
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: backstage.Name, Namespace: object.GetNamespace()}}}

}

// SetupWithManager sets up the controller with the Manager.
func (r *BackstageReconciler) SetupWithManager(mgr ctrl.Manager) error {

	pred, err := predicate.LabelSelectorPredicate(watchedConfigSelector)
	if err != nil {
		return fmt.Errorf("failed to construct the predicate for matching secrets. This should not happen: %w", err)
	}

	secretMeta := &metav1.PartialObjectMetadata{}
	secretMeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Secret",
	})

	configMapMeta := &metav1.PartialObjectMetadata{}
	configMapMeta.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMap",
	})

	b := ctrl.NewControllerManagedBy(mgr).
		For(&bs.Backstage{}).
		WatchesMetadata(
			secretMeta,
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
				return r.requestByLabel(ctx, o)
			}),
			builder.WithPredicates(pred, predicate.Funcs{
				DeleteFunc: func(e event.DeleteEvent) bool { return true },
				UpdateFunc: func(e event.UpdateEvent) bool { return true },
				//CreateFunc: func(e event.CreateEvent) bool { return true },
			}),
		).
		WatchesMetadata(
			configMapMeta,
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
				return r.requestByLabel(ctx, o)
			}),
			builder.WithPredicates(pred, predicate.Funcs{
				DeleteFunc: func(e event.DeleteEvent) bool { return true },
				UpdateFunc: func(e event.UpdateEvent) bool { return true },
				//CreateFunc: func(e event.CreateEvent) bool { return true },
			}))

	return b.Complete(r)
}
