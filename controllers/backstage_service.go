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
			Name:      getDefaultObjName(backstage),
			Namespace: ns,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, r.serviceObjectMutFun(ctx, service, backstage,
		backstage.Spec.RawRuntimeConfig.BackstageConfigName, "service.yaml", service.Name, service.Name)); err != nil {
		if errors.IsConflict(err) {
			return fmt.Errorf("retry sync needed: %v", err)
		}
		return err
	}
	return nil
}

// selector for deploy.spec.template.spec.meta.label
// targetPort: http for deploy.spec.template.spec.containers.ports.name=http
func (r *BackstageReconciler) serviceObjectMutFun(ctx context.Context, targetService *corev1.Service, backstage bs.Backstage,
	configName, configKey, serviceName, label string) controllerutil.MutateFn {
	return func() error {
		service := &corev1.Service{}
		targetService.ObjectMeta.DeepCopyInto(&service.ObjectMeta)

		err := r.readConfigMapOrDefault(ctx, configName, configKey, backstage.Namespace, service)
		if err != nil {
			return err
		}

		service.Name = serviceName
		setLabel(&service.ObjectMeta.Labels, label)
		setLabel(&service.Spec.Selector, label)
		r.labels(&service.ObjectMeta, backstage)

		if r.OwnsRuntime {
			if err := controllerutil.SetControllerReference(&backstage, service, r.Scheme); err != nil {
				return fmt.Errorf("failed to set owner reference: %s", err)
			}
		}

		if err := validateServiceIPs(targetService, service); err != nil {
			return err
		}
		service.Spec.ClusterIPs = targetService.Spec.ClusterIPs

		service.ObjectMeta.DeepCopyInto(&targetService.ObjectMeta)
		service.Spec.DeepCopyInto(&targetService.Spec)
		return nil
	}
}

func validateServiceIPs(targetService *corev1.Service, service *corev1.Service) error {
	if len(targetService.Spec.ClusterIP) > 0 && service.Spec.ClusterIP != "" && service.Spec.ClusterIP != "None" && service.Spec.ClusterIP != targetService.Spec.ClusterIP {
		return fmt.Errorf("backstage service IP can not be updated: %s, current: %s, new: %s", targetService.Name, targetService.Spec.ClusterIP, service.Spec.ClusterIP)
	}
	service.Spec.ClusterIP = targetService.Spec.ClusterIP
	for _, ip1 := range targetService.Spec.ClusterIPs {
		for _, ip2 := range service.Spec.ClusterIPs {
			if len(ip1) > 0 && ip2 != "" && ip2 != "None" && ip1 != ip2 {
				return fmt.Errorf("backstage service IPs can not be updated: %s, current: %v, new: %v", targetService.Name, targetService.Spec.ClusterIPs, service.Spec.ClusterIPs)
			}
		}
	}
	return nil
}
