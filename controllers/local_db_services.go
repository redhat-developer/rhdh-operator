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

	bs "janus-idp.io/backstage-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// var (`
//
//	DefaultLocalDbService = `apiVersion: v1
//
// kind: Service
// metadata:
//
//	name: backstage-psql-cr1 # placeholder for 'backstage-psql-<cr-name>'
//
// spec:
//
//	selector:
//	    janus-idp.io/app:  backstage-psql-cr1 # placeholder for 'backstage-psql-<cr-name>'
//	ports:
//	  - port: 5432
//
// `
//
//	DefaultLocalDbServiceHL = `apiVersion: v1
//
// kind: Service
// metadata:
//
//	name: backstage-psql-cr1-hl # placeholder for 'backstage-psql-<cr-name>-hl'
//
// spec:
//
//	selector:
//	    janus-idp.io/app:  backstage-psql-cr1 # placeholder for 'backstage-psql-<cr-name>'
//	clusterIP: None
//	ports:
//	  - port: 5432
//
// `
// )
func (r *BackstageReconciler) reconcileLocalDbServices(ctx context.Context, backstage bs.Backstage, ns string) error {
	name := fmt.Sprintf("backstage-psql-%s", backstage.Name)
	err := r.reconcilePsqlService(ctx, backstage, name, name, ns, "db-service.yaml")
	if err != nil {
		return err
	}
	nameHL := fmt.Sprintf("backstage-psql-%s-hl", backstage.Name)
	return r.reconcilePsqlService(ctx, backstage, nameHL, name, ns, "db-service-hl.yaml")

}

func (r *BackstageReconciler) reconcilePsqlService(ctx context.Context, backstage bs.Backstage, name, label, ns, key string) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, r.psqlServiceObjectMutFun(ctx, service, backstage, label, ns, key)); err != nil {
		if errors.IsConflict(err) {
			return fmt.Errorf("retry sync needed: %v", err)
		}
		return err
	}
	return nil
}

func (r *BackstageReconciler) psqlServiceObjectMutFun(ctx context.Context, targetService *corev1.Service, backstage bs.Backstage, label, ns, key string) controllerutil.MutateFn {
	return func() error {
		service := &corev1.Service{}
		targetService.ObjectMeta.DeepCopyInto(&service.ObjectMeta)
		err := r.readConfigMapOrDefault(ctx, backstage.Spec.RawRuntimeConfig.LocalDbConfigName, key, ns, service)
		if err != nil {
			return err
		}
		service.SetName(targetService.Name)
		setBackstageLocalDbLabel(&service.ObjectMeta.Labels, label)
		setBackstageLocalDbLabel(&service.Spec.Selector, label)

		if r.OwnsRuntime {
			if err := controllerutil.SetControllerReference(&backstage, service, r.Scheme); err != nil {
				return fmt.Errorf(ownerRefFmt, err)
			}
		}
		if len(targetService.Spec.ClusterIP) > 0 && service.Spec.ClusterIP != "" && service.Spec.ClusterIP != "None" && service.Spec.ClusterIP != targetService.Spec.ClusterIP {
			return fmt.Errorf("db service IP can not be updated: %s, %s, %s", targetService.Name, targetService.Spec.ClusterIP, service.Spec.ClusterIP)
		}
		service.Spec.ClusterIP = targetService.Spec.ClusterIP
		for _, ip1 := range targetService.Spec.ClusterIPs {
			for _, ip2 := range service.Spec.ClusterIPs {
				if len(ip1) > 0 && ip2 != "" && ip2 != "None" && ip1 != ip2 {
					return fmt.Errorf("db service IPs can not be updated: %s, %v, %v", targetService.Name, targetService.Spec.ClusterIPs, service.Spec.ClusterIPs)
				}
			}
		}
		service.Spec.ClusterIPs = targetService.Spec.ClusterIPs

		service.ObjectMeta.DeepCopyInto(&targetService.ObjectMeta)
		service.Spec.DeepCopyInto(&targetService.Spec)
		return nil
	}
}
