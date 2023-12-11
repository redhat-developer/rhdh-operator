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
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	bs "janus-idp.io/backstage-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	BackstageAppLabel = "janus-idp.io/app"
)

// BackstageReconciler reconciles a Backstage object
type BackstageReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// If true, Backstage Controller always sync the state of runtime objects created
	// otherwise, runtime objects can be re-configured independently
	OwnsRuntime bool

	// Namespace allows to restrict the reconciliation to this particular namespace,
	// and ignore requests from other namespaces.
	// This is mostly useful for our tests, to overcome a limitation of EnvTest about namespace deletion.
	Namespace string
}

//+kubebuilder:rbac:groups=janus-idp.io,resources=backstages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=janus-idp.io,resources=backstages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=janus-idp.io,resources=backstages/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=configmaps;secrets;persistentvolumes;persistentvolumeclaims;services,verbs=get;watch;create;update;list;delete
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;watch;create;update;list;delete
//+kubebuilder:rbac:groups="apps",resources=statefulsets,verbs=get;watch;create;update;list;delete

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

	lg.V(1).Info(fmt.Sprintf("starting reconciliation (namespace: %q)", req.NamespacedName))

	// Ignore requests for other namespaces, if specified.
	// This is mostly useful for our tests, to overcome a limitation of EnvTest about namespace deletion.
	// More details on https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
	if r.Namespace != "" && req.Namespace != r.Namespace {
		return ctrl.Result{}, nil
	}

	backstage := bs.Backstage{}
	if err := r.Get(ctx, req.NamespacedName, &backstage); err != nil {
		if errors.IsNotFound(err) {
			lg.Info("backstage gone from the namespace")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to load backstage deployment from the cluster: %w", err)
	}

	if pointer.BoolDeref(backstage.Spec.EnableLocalDb, true) {

		/* We use default strogeclass currently, and no PV is needed in that case.
		If we decide later on to support user provided storageclass we can enable pv creation.
		if err := r.applyPV(ctx, backstage, req.Namespace); err != nil {
			return ctrl.Result{}, err
		}
		*/

		err := r.applyLocalDbStatefulSet(ctx, backstage, req.Namespace)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to apply Database StatefulSet: %w", err)
		}

		err = r.applyLocalDbServices(ctx, backstage, req.Namespace)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to apply Database Service: %w", err)
		}

	}

	err := r.applyBackstageDeployment(ctx, backstage, req.Namespace)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to apply Backstage Deployment: %w", err)
	}

	if err := r.applyBackstageService(ctx, backstage, req.Namespace); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to apply Backstage Service: %w", err)
	}

	//TODO: it is just a placeholder for the time
	r.setRunningStatus(ctx, &backstage, req.Namespace)
	r.setSyncStatus(&backstage)
	err = r.Status().Update(ctx, &backstage)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set status: %w", err)
		//log.FromContext(ctx).Error(err, "unable to update backstage.status")
	}

	return ctrl.Result{}, nil
}

func (r *BackstageReconciler) readConfigMapOrDefault(ctx context.Context, name string, key string, ns string, object v1.Object) error {

	lg := log.FromContext(ctx)

	if name == "" {
		err := readYamlFile(defFile(key), object)
		if err != nil {
			return fmt.Errorf("failed to read YAML file: %w", err)
		}
		object.SetNamespace(ns)
		return nil
	}

	cm := corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, &cm); err != nil {
		return err
	}

	val, ok := cm.Data[key]
	if !ok {
		// key not found, default
		lg.V(1).Info("custom configuration configMap and data exists, trying to apply it", "configMap", cm.Name, "key", key)
		err := readYamlFile(defFile(key), object)
		if err != nil {
			return fmt.Errorf("failed to read YAML file: %w", err)
		}
	} else {
		lg.V(1).Info("custom configuration configMap exists but no such key, applying default config", "configMap", cm.Name, "key", key)
		err := readYaml([]byte(val), object)
		if err != nil {
			return fmt.Errorf("failed to read YAML: %w", err)
		}
	}
	object.SetNamespace(ns)
	return nil
}

func readYaml(manifest []byte, object interface{}) error {
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifest), 1000)
	if err := dec.Decode(object); err != nil {
		return fmt.Errorf("failed to decode YAML: %w", err)
	}
	return nil
}

func readYamlFile(path string, object interface{}) error {

	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read YAML file: %w", err)
	}
	return readYaml(b, object)
}

func defFile(key string) string {
	return filepath.Join(os.Getenv("LOCALBIN"), "default-config", key)
}

// sets the RuntimeRunning condition
func (r *BackstageReconciler) setRunningStatus(ctx context.Context, backstage *bs.Backstage, ns string) {

	meta.SetStatusCondition(&backstage.Status.Conditions, v1.Condition{
		Type:               bs.RuntimeConditionRunning,
		Status:             "Unknown",
		LastTransitionTime: v1.Time{},
		Reason:             "Unknown",
		Message:            "Runtime in unknown status",
	})
}

// sets the RuntimeSyncedWithConfig condition
func (r *BackstageReconciler) setSyncStatus(backstage *bs.Backstage) {

	status := v1.ConditionUnknown
	reason := "Unknown"
	message := "Sync in unknown status"
	if r.OwnsRuntime {
		status = v1.ConditionTrue
		reason = "Synced"
		message = "Backstage syncs runtime"
	}

	meta.SetStatusCondition(&backstage.Status.Conditions, v1.Condition{
		Type:               bs.RuntimeConditionSynced,
		Status:             status,
		LastTransitionTime: v1.Time{},
		Reason:             reason,
		Message:            message,
	})
}

// sets backstage-{Id} for labels and selectors
func setBackstageAppLabel(labels *map[string]string, backstage bs.Backstage) {
	if *labels == nil {
		*labels = map[string]string{}
	}
	(*labels)[BackstageAppLabel] = fmt.Sprintf("backstage-%s", backstage.Name)
}

// sets backstage-psql-{Id} for labels and selectors
func setBackstageLocalDbLabel(labels *map[string]string, name string) {
	if *labels == nil {
		*labels = map[string]string{}
	}
	(*labels)[BackstageAppLabel] = name
}

// sets labels on Backstage's instance resources
func (r *BackstageReconciler) labels(meta *v1.ObjectMeta, backstage bs.Backstage) {
	if meta.Labels == nil {
		meta.Labels = map[string]string{}
	}
	meta.Labels["app.kubernetes.io/name"] = "backstage"
	meta.Labels["app.kubernetes.io/instance"] = backstage.Name
	//meta.Labels[BackstageAppLabel] = fmt.Sprintf("backstage-%s", backstage.Name)
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackstageReconciler) SetupWithManager(mgr ctrl.Manager) error {

	//if err := initDefaults(); err != nil {
	//	return err
	//}
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&bs.Backstage{})

	if r.OwnsRuntime {
		builder.Owns(&appsv1.Deployment{}).
			Owns(&corev1.Service{}).
			Owns(&corev1.PersistentVolume{}).
			Owns(&corev1.PersistentVolumeClaim{})
	}

	return builder.Complete(r)
}
