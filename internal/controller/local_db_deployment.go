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
	bs "backstage.io/backstage-deploy-operator/api/v1alpha1"
	"context"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	DefaultLocalDbDeployment = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgres  # backstage-db-<cr-name>
  template:
    metadata:
      labels:
        app: postgres # backstage-db-<cr-name>
    spec:
      containers:
        - name: postgres
          image: postgres:13.2-alpine
          imagePullPolicy: 'IfNotPresent'
          ports:
            - containerPort: 5432
          envFrom:
            - secretRef:
                name: postgres-secrets
          volumeMounts:
            - mountPath: /var/lib/postgresql/data
              name: postgresdb
      volumes:
        - name: postgresdb
          persistentVolumeClaim:
            claimName: postgres-storage-claim
`
	DefaultLocalDbService = `apiVersion: v1
kind: Service
metadata:
  name: postgres
spec:
  selector:
    app: postgres # backstage-db-<cr-name>
  ports:
    - port: 5432
`
)

func (r *BackstageReconciler) applyLocalDbDeployment(ctx context.Context, backstage bs.Backstage, ns string) error {

	lg := log.FromContext(ctx)

	deployment := &appsv1.Deployment{}
	err := r.readConfigMapOrDefault(ctx, backstage.Spec.RuntimeConfig.LocalDbConfigName, "deployment", ns, DefaultLocalDbDeployment, deployment)
	if err != nil {
		return err
	}
	deployment.Spec.Template.ObjectMeta.Labels["app"] = fmt.Sprintf("backstage-db-%s", backstage.Name)
	deployment.Spec.Selector.MatchLabels["app"] = fmt.Sprintf("backstage-db-%s", backstage.Name)

	//deployment.Namespace = ns
	err = r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: ns}, deployment)
	if err != nil {
		if errors.IsNotFound(err) {

		} else {
			return fmt.Errorf("failed to get deployment, reason: %s", err)
		}
	} else {
		lg.Info("CR update is ignored for the time")
		return nil
	}

	if !backstage.Spec.DryRun {
		err = r.Create(ctx, deployment)
		if err != nil {
			return fmt.Errorf("failed to create deplyment, reason: %s", err)
		}
	}

	return nil
}

func (r *BackstageReconciler) applyLocalDbService(ctx context.Context, backstage bs.Backstage, ns string) error {

	lg := log.FromContext(ctx)

	service := &corev1.Service{}
	err := r.readConfigMapOrDefault(ctx, backstage.Spec.RuntimeConfig.LocalDbConfigName, "service", ns, DefaultLocalDbService, service)
	if err != nil {
		return err
	}
	service.Spec.Selector["app"] = fmt.Sprintf("backstage-db-%s", backstage.Name)

	//service.Namespace = ns
	err = r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: ns}, service)
	if err != nil {
		if errors.IsNotFound(err) {
		} else {
			return fmt.Errorf("failed to get service, reason: %s", err)
		}
	} else {
		lg.Info("CR update is ignored for the time")
		return nil
	}

	if !backstage.Spec.DryRun {
		err = r.Create(ctx, service)
		if err != nil {
			return fmt.Errorf("failed to create service, reason: %s", err)
		}
	}

	return nil
}
