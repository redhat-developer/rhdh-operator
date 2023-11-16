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

	bs "backstage.io/backstage-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	DefaultLocalDbPV = `
apiVersion: v1
kind: PersistentVolume
metadata:
  name: postgres-storage
  namespace: backstage
  labels:
    type: local
spec:
  storageClassName: manual
  capacity:
    storage: 2G
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  hostPath:
    path: '/mnt/data'
`
	DefaultLocalDbPVC = `
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-storage-claim
spec:
  storageClassName: manual
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 2G
`
)

func (r *BackstageReconciler) applyPV(ctx context.Context, backstage bs.Backstage, ns string) error {
	// Postgre PersistentVolume
	lg := log.FromContext(ctx)

	pv := &corev1.PersistentVolume{}
	err := r.readConfigMapOrDefault(ctx, backstage.Spec.RuntimeConfig.LocalDbConfigName, "persistentVolume", ns, DefaultLocalDbPV, pv)
	if err != nil {
		return err
	}

	//pv.Namespace = ns
	err = r.Get(ctx, types.NamespacedName{Name: pv.Name, Namespace: ns}, pv)

	if err != nil {
		if errors.IsNotFound(err) {
		} else {
			return fmt.Errorf("failed to get PV, reason: %s", err)
		}
	} else {
		lg.Info("CR update is ignored for the time")
		return nil
	}

	r.labels(pv.ObjectMeta, backstage.Name)
	if !backstage.Spec.DryRun {
		err = r.Create(ctx, pv)
		if err != nil {
			//status = fmt.Sprintf("failed to create postgre persistent volume, reason:%s", err)
			return fmt.Errorf("failed to create postgre persistent volume, reason:%s", err)
		}
	}

	return nil
}

func (r *BackstageReconciler) applyPVC(ctx context.Context, backstage bs.Backstage, ns string) error {
	// Postgre PersistentVolumeClaim
	lg := log.FromContext(ctx)

	pvc := &corev1.PersistentVolumeClaim{}
	err := r.readConfigMapOrDefault(ctx, backstage.Spec.RuntimeConfig.LocalDbConfigName, "persistentVolumeClaim", ns, DefaultLocalDbPVC, pvc)
	if err != nil {
		return err
	}

	//pvc.Namespace = ns
	err = r.Get(ctx, types.NamespacedName{Name: pvc.Name, Namespace: ns}, pvc)

	if err != nil {
		if errors.IsNotFound(err) {
		} else {
			return fmt.Errorf("failed to get PVC, reason: %s", err)
		}
	} else {
		lg.Info("CR update is ignored for the time")
		return nil
	}

	r.labels(pvc.ObjectMeta, backstage.Name)
	if !backstage.Spec.DryRun {
		err = r.Create(ctx, pvc)
		if err != nil {
			//status = fmt.Sprintf("failed to create postgre persistent volume, reason:%s", err)
			return fmt.Errorf("failed to create postgre persistent volume claim, reason:%s", err)
		}
	}

	return nil
}
