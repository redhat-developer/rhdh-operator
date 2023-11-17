/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	bs "backstage.io/backstage-operator/api/v1alpha1"
	"bytes"
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// BackstageReconciler reconciles a Backstage object
type BackstageReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=backstage.io,resources=backstages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=backstage.io,resources=backstages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=backstage.io,resources=backstages/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=configmaps;persistentvolumes;persistentvolumeclaims;services,verbs=get;watch;create;update;list;delete
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;watch;create;update;list;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Backstage object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *BackstageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx)

	lg.V(1).Info("starting reconciliation")

	backstage := bs.Backstage{}
	if err := r.Get(ctx, req.NamespacedName, &backstage); err != nil {
		if errors.IsNotFound(err) {
			lg.Info("backstage gone from the namespace")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to load backstage deployment from the cluster: %w", err)
	}

	if !backstage.Spec.SkipLocalDb {
		// log Debug
		if err := r.applyPV(ctx, backstage, req.Namespace); err != nil {
			//backstage.Status.LocalDb.PersistentVolume.Status = err.Error()
			return ctrl.Result{}, err
		}

		if err := r.applyPVC(ctx, backstage, req.Namespace); err != nil {
			//backstage.Status.PostgreState = err.Error()
			return ctrl.Result{}, err
		}

		err := r.applyLocalDbDeployment(ctx, backstage, req.Namespace)
		if err != nil {

			//backstage.Status.PostgreState = err.Error()
			return ctrl.Result{}, err
		}

		err = r.applyLocalDbService(ctx, backstage, req.Namespace)
		if err != nil {
			//backstage.Status.PostgreState = err.Error()
			return ctrl.Result{}, err
		}

	}

	if err := r.applyBackstageDeployment(ctx, backstage, req.Namespace); err != nil {
		// TODO BackstageDepState state
		//backstage.Status.BackstageState = err.Error()
		return ctrl.Result{}, err
	}

	if err := r.applyBackstageService(ctx, backstage, req.Namespace); err != nil {
		// TODO BackstageDepState state
		//backstage.Status.BackstageState = err.Error()
		return ctrl.Result{}, err
	}

	backstage.Status.BackstageState = "deployed"
	r.Status().Update(ctx, &backstage)

	//return ctrl.Result{Requeue: !r.checkPostgreSecret(ctx, backstage, req.Namespace)}, nil
	return ctrl.Result{}, nil
}

func (r *BackstageReconciler) readConfigMapOrDefault(ctx context.Context, name string, key string, ns string, def string, object v1.Object) error {

	// ConfigMap name not set, default
	lg := log.FromContext(ctx)

	lg.V(1).Info("readConfigMapOrDefault CM: ", "name", name)

	if name == "" {
		err := readYaml(def, object)
		if err != nil {
			return err
		}
		object.SetNamespace(ns)
		return nil
	}

	cm := corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, &cm); err != nil {
		return err
	}
	lg.V(1).Info("readConfigMapOrDefault CM name found: ", "ConfigMap:", cm)
	val, ok := cm.Data[key]
	if !ok {
		// key not found, default
		err := readYaml(def, object)
		if err != nil {
			return err
		}
	} else {
		err := readYaml(val, object)
		if err != nil {
			return err
		}
	}
	object.SetNamespace(ns)
	return nil
}

func (r *BackstageReconciler) labels(meta v1.ObjectMeta, name string) {
	if meta.Labels == nil {
		meta.Labels = map[string]string{}
	}
	meta.Labels["app.kubernetes.io/name"] = "backstage"
	meta.Labels["app.kubernetes.io/instance"] = name
}

func readYaml(manifest string, object interface{}) error {
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(manifest)), 1000)
	if err := dec.Decode(object); err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackstageReconciler) SetupWithManager(mgr ctrl.Manager) error {

	//if err := initDefaults(); err != nil {
	//	return err
	//}
	return ctrl.NewControllerManagedBy(mgr).
		For(&bs.Backstage{}).
		Complete(r)
}

//func initDefaults() error {
//	//deployment = &appsv1.Deployment{}
//	if err := readYaml(DefaultBackstageDeployment, DefBackstageDeployment); err != nil {
//		return err
//	}
//
//	return nil
//}
