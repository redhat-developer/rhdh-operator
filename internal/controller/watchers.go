package controller

import (
	"context"
	"fmt"

	bs "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
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
				//CreateFunc: func(e event.CreateEvent) bool { return true },
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
					//CreateFunc: func(e event.CreateEvent) bool { return true },
				}))
	}

	// Watch Deployment for the Backstage CR status if enabled
	labelPred, err := predicate.LabelSelectorPredicate(metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      utils.BackstageAppLabel,
				Values:   []string{utils.BackstageAppName},
				Operator: metav1.LabelSelectorOpIn,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to construct the predicate for backstage deployment. This should not happen: %w", err)
	}

	b.WatchesMetadata(
		&metav1.PartialObjectMetadata{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
		},
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
			return r.requestByAppLabels(ctx, o)
		}),
		builder.WithPredicates(labelPred,
			predicate.Funcs{
				DeleteFunc: func(e event.DeleteEvent) bool { return true },
				UpdateFunc: func(e event.UpdateEvent) bool { return true },
				CreateFunc: func(e event.CreateEvent) bool { return true },
			}),
	)
	return nil
}

// requestByExtConfigLabel returns a request with current Namespace and Backstage Object name taken from label
// or empty request object if label not found
func (r *BackstageReconciler) requestByExtConfigLabel(ctx context.Context, object client.Object) []reconcile.Request {

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

	newHash := ec.WatchingHash
	oldHash := deploy.Spec.Template.ObjectMeta.GetAnnotations()[model.ExtConfigHashAnnotation]
	if newHash == oldHash {
		lg.V(1).Info("request by label, hash are equal", "hash", newHash)
		return []reconcile.Request{}
	}

	lg.V(1).Info("enqueuing reconcile for", object.GetObjectKind().GroupVersionKind().Kind, object.GetName(), "new hash: ", newHash, "old hash: ", oldHash)
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: backstage.Name, Namespace: object.GetNamespace()}}}

}

// requestByAppLabels returns a request with current Namespace and Backstage Object name taken from label
func (r *BackstageReconciler) requestByAppLabels(ctx context.Context, object client.Object) []reconcile.Request {
	lg := log.FromContext(ctx)

	backstageName := object.GetLabels()[utils.BackstageInstanceLabel]
	if object.GetLabels()[utils.BackstageAppLabel] == "" || backstageName == "" {
		return []reconcile.Request{}
	}

	lg.V(1).Info("enqueuing reconcile on Deployment change", object.GetObjectKind().GroupVersionKind().Kind, object.GetName(), "namespace: ", object.GetNamespace())
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: backstageName, Namespace: object.GetNamespace()}}}
}
