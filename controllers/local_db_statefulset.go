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
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	bs "janus-idp.io/backstage-operator/api/v1alpha1"
)

const (
	ownerRefFmt = "failed to set owner reference: %s"
)

func (r *BackstageReconciler) reconcileLocalDbStatefulSet(ctx context.Context, backstage *bs.Backstage, ns string) error {
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getDefaultDbObjName(*backstage),
			Namespace: ns,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, statefulSet, r.localDBStatefulSetMutFun(ctx, statefulSet, *backstage, ns)); err != nil {
		if errors.IsConflict(err) {
			return retryReconciliation(err)
		}
		msg := fmt.Sprintf("failed to deploy Database StatefulSet: %s", err)
		setStatusCondition(backstage, bs.ConditionDeployed, metav1.ConditionFalse, bs.DeployFailed, msg)
		return fmt.Errorf(msg)
	}
	return nil
}

func (r *BackstageReconciler) localDBStatefulSetMutFun(ctx context.Context, targetStatefulSet *appsv1.StatefulSet, backstage bs.Backstage, ns string) controllerutil.MutateFn {
	return func() error {
		statefulSet := &appsv1.StatefulSet{}
		targetStatefulSet.ObjectMeta.DeepCopyInto(&statefulSet.ObjectMeta)
		err := r.readConfigMapOrDefault(ctx, backstage.Spec.RawRuntimeConfig.LocalDbConfigName, "db-statefulset.yaml", ns, statefulSet)
		if err != nil {
			return err
		}

		// Override the name
		statefulSet.Name = getDefaultDbObjName(backstage)
		if err = r.patchLocalDbStatefulSetObj(statefulSet, backstage); err != nil {
			return err
		}
		r.labels(&statefulSet.ObjectMeta, backstage)
		if err = r.patchLocalDbStatefulSetObj(statefulSet, backstage); err != nil {
			return err
		}

		r.setDefaultStatefulSetImage(statefulSet)

		_, err = r.handlePsqlSecret(ctx, statefulSet, &backstage)
		if err != nil {
			return err
		}

		if r.OwnsRuntime {
			// Set the ownerreferences for the statefulset so that when the backstage CR is deleted,
			// the statefulset is automatically deleted
			// Note that the PVCs associated with the statefulset are not deleted automatically
			// to prevent data loss. However OpenShift v4.14 and Kubernetes v1.27 introduced an optional
			// parameter persistentVolumeClaimRetentionPolicy in the statefulset spec:
			// spec:
			//   persistentVolumeClaimRetentionPolicy:
			//     whenDeleted: Delete
			//     whenScaled: Retain
			// This will allow the PVCs to get automatically deleted when the statefulset is deleted if
			// the StatefulSetAutoDeletePVC feature gate is enabled on the API server.
			// For more information, see https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/
			if err := controllerutil.SetControllerReference(&backstage, statefulSet, r.Scheme); err != nil {
				return fmt.Errorf(ownerRefFmt, err)
			}
		}

		statefulSet.ObjectMeta.DeepCopyInto(&targetStatefulSet.ObjectMeta)
		statefulSet.Spec.DeepCopyInto(&targetStatefulSet.Spec)
		return nil
	}
}

func (r *BackstageReconciler) patchLocalDbStatefulSetObj(statefulSet *appsv1.StatefulSet, backstage bs.Backstage) error {
	name := getDefaultDbObjName(backstage)
	statefulSet.SetName(name)
	statefulSet.Spec.Template.SetName(name)
	statefulSet.Spec.ServiceName = fmt.Sprintf("%s-hl", name)

	setLabel(&statefulSet.Spec.Template.ObjectMeta.Labels, name)
	setLabel(&statefulSet.Spec.Selector.MatchLabels, name)

	return nil
}

func (r *BackstageReconciler) setDefaultStatefulSetImage(statefulSet *appsv1.StatefulSet) {
	if envPostgresImage != "" {
		visitContainers(&statefulSet.Spec.Template, func(container *v1.Container) {
			container.Image = envPostgresImage
		})
	}
}

// cleanupLocalDbResources removes all local db related resources, including statefulset, services and generated secret.
func (r *BackstageReconciler) cleanupLocalDbResources(ctx context.Context, backstage bs.Backstage) error {
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getDefaultDbObjName(backstage),
			Namespace: backstage.Namespace,
		},
	}
	if _, err := r.cleanupResource(ctx, statefulSet, backstage); err != nil {
		return fmt.Errorf("failed to delete database statefulset, reason: %s", err)
	}

	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getDefaultDbObjName(backstage),
			Namespace: backstage.Namespace,
		},
	}
	if _, err := r.cleanupResource(ctx, service, backstage); err != nil {
		return fmt.Errorf("failed to delete database service, reason: %s", err)
	}
	serviceHL := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("backstage-psql-%s-hl", backstage.Name),
			Namespace: backstage.Namespace,
		},
	}
	if _, err := r.cleanupResource(ctx, serviceHL, backstage); err != nil {
		return fmt.Errorf("failed to delete headless database service, reason: %s", err)
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getDefaultPsqlSecretName(&backstage),
			Namespace: backstage.Namespace,
		},
	}
	if _, err := r.cleanupResource(ctx, secret, backstage); err != nil {
		return fmt.Errorf("failed to delete generated database secret, reason: %s", err)
	}
	return nil
}
