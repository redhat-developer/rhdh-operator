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

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	bs "janus-idp.io/backstage-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *BackstageReconciler) reconcileBackstageService(ctx context.Context, backstage bs.Backstage, ns string) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("backstage-%s", backstage.Name),
			Namespace: ns,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, r.serviceObjectMutFun(ctx, service, backstage, ns)); err != nil {
		if errors.IsConflict(err) {
			return fmt.Errorf("retry sync needed: %v", err)
		}
		return err
	}
	return nil
}

// selector for deploy.spec.template.spec.meta.label
// targetPort: http for deploy.spec.template.spec.containers.ports.name=http
func (r *BackstageReconciler) serviceObjectMutFun(ctx context.Context, service *corev1.Service, backstage bs.Backstage, ns string) controllerutil.MutateFn {
	return func() error {
		tmp := service.DeepCopy()
		err := r.readConfigMapOrDefault(ctx, backstage.Spec.RawRuntimeConfig.BackstageConfigName, "service.yaml", ns, service)
		if err != nil {
			return err
		}

		// Override the service name
		service.Name = fmt.Sprintf("backstage-%s", backstage.Name)
		setBackstageAppLabel(&service.Spec.Selector, backstage)

		r.labels(&service.ObjectMeta, backstage)

		if r.OwnsRuntime {
			if err := controllerutil.SetControllerReference(&backstage, service, r.Scheme); err != nil {
				return fmt.Errorf("failed to set owner reference: %s", err)
			}
		}

		if len(tmp.Spec.ClusterIP) > 0 && service.Spec.ClusterIP != "" && service.Spec.ClusterIP != "None" && service.Spec.ClusterIP != tmp.Spec.ClusterIP {
			return fmt.Errorf("backstage service IP can not be updated: %s, %s, %s", tmp.Name, tmp.Spec.ClusterIP, service.Spec.ClusterIP)
		}
		service.Spec.ClusterIP = tmp.Spec.ClusterIP
		for _, ip1 := range tmp.Spec.ClusterIPs {
			for _, ip2 := range service.Spec.ClusterIPs {
				if len(ip1) > 0 && ip2 != "" && ip2 != "None" && ip1 != ip2 {
					return fmt.Errorf("backstage service IPs can not be updated: %s, %v, %v", tmp.Name, tmp.Spec.ClusterIPs, service.Spec.ClusterIPs)
				}
			}
		}
		service.Spec.ClusterIPs = tmp.Spec.ClusterIPs
		return nil
	}
}
