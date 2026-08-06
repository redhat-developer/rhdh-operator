package controller

import (
	"context"
	"fmt"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *BackstageReconciler) addWatchers(b *builder.Builder) error {
	// Watch in all the cases but WatchExtConfig == false
	if utils.BoolEnvVar(WatchExtConfig, true) {

		pred, err := predicate.LabelSelectorPredicate(metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      model.ExtConfigSyncLabel,
					Values:   []string{"true"},
					Operator: metav1.LabelSelectorOpIn,
				},
			},
		})
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

		b.WatchesMetadata(
			secretMeta,
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
				return r.requestByExtConfigLabel(ctx, o)
			}),
			builder.WithPredicates(pred, predicate.Funcs{
				DeleteFunc: func(e event.DeleteEvent) bool { return true },
				UpdateFunc: func(e event.UpdateEvent) bool { return true },
				// CreateFunc: func(e event.CreateEvent) bool { return true },
			}),
		).
			WatchesMetadata(
				configMapMeta,
				handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
					return r.requestByExtConfigLabel(ctx, o)
				}),
				builder.WithPredicates(pred, predicate.Funcs{
					DeleteFunc: func(e event.DeleteEvent) bool { return true },
					UpdateFunc: func(e event.UpdateEvent) bool { return true },
					// CreateFunc: func(e event.CreateEvent) bool { return true },
				}))
	}

	// Watch operator-owned Deployments and StatefulSets for status tracking.
	// Owns() maps events back to the owning Backstage CR via ownerReferences.
	b.Owns(&appsv1.Deployment{}).
		Owns(&appsv1.StatefulSet{})

	return nil
}

// requestByExtConfigLabel returns a request with current Namespace and Backstage Object name taken from label
// or empty request object if label not found
func (r *BackstageReconciler) requestByExtConfigLabel(ctx context.Context, object client.Object) []reconcile.Request {

	lg := log.FromContext(ctx)

	backstageName := object.GetAnnotations()[model.BackstageNameAnnotation]
	if backstageName == "" {
		// lg.V(1).Info(fmt.Sprintf("warning: %s annotation is not defined for %s, Backstage instances will not be reconciled in this loop", model.BackstageNameAnnotation, object.GetName()))
		return []reconcile.Request{}
	}

	nn := types.NamespacedName{
		Namespace: object.GetNamespace(),
		Name:      backstageName,
	}

	backstage := api.Backstage{}
	if err := r.Get(ctx, nn, &backstage); err != nil {
		if !errors.IsNotFound(err) {
			lg.Error(err, "request by label failed, get Backstage ")
		}
		return []reconcile.Request{}
	}

	// ec, err := r.preprocessSpec(ctx, backstage)
	// if err != nil {
	// 	lg.Error(err, "request by label failed, preprocess Backstage ")
	// 	return []reconcile.Request{}
	// }

	deploy, err := FindDeployment(ctx, r.Client, object.GetNamespace(), backstage.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			lg.V(1).Info("request by label could not find a resource (most likely not created yet)", "error", err.Error())
		} else {
			lg.Error(err, "request by label failed, find deployment ")
		}
		return []reconcile.Request{}
	}

	ec, err := r.preprocessSpec(ctx, backstage)
	if err != nil {
		lg.Error(err, "request by label failed, preprocess Backstage ")
		return []reconcile.Request{}
	}

	newHash := ec.WatchingHash
	oldHash := deploy.GetObject().GetAnnotations()[model.ExtConfigHashAnnotation]
	if newHash == oldHash {
		lg.V(1).Info("request by label, hash are equal", "hash", newHash)
		return []reconcile.Request{}
	}

	lg.V(1).Info("enqueuing reconcile for", object.GetObjectKind().GroupVersionKind().Kind, object.GetName(), "new hash: ", newHash, "old hash: ", oldHash)
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: backstage.Name, Namespace: object.GetNamespace()}}}

}
