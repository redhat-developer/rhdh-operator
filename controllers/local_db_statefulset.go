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

//var (
//	DefaultLocalDbDeployment = `apiVersion: apps/v1
//kind: StatefulSet
//metadata:
//  name: backstage-psql-cr1 # placeholder for 'backstage-psql-<cr-name>'
//spec:
//  podManagementPolicy: OrderedReady
//  replicas: 1
//  selector:
//    matchLabels:
//      janus-idp.io/app: backstage-psql-cr1 # placeholder for 'backstage-psql-<cr-name>'
//  serviceName: backstage-psql-cr1-hl # placeholder for 'backstage-psql-<cr-name>-hl'
//  template:
//    metadata:
//      labels:
//        janus-idp.io/app: backstage-psql-cr1 # placeholder for 'backstage-psql-<cr-name>'
//      name: backstage-db-cr1 # placeholder for 'backstage-psql-<cr-name>'
//    spec:
//      containers:
//      - env:
//        - name: POSTGRESQL_PORT_NUMBER
//          value: "5432"
//        - name: POSTGRESQL_VOLUME_DIR
//          value: /var/lib/pgsql/data
//        - name: PGDATA
//          value: /var/lib/pgsql/data/userdata
//        envFrom:
//          - secretRef:
//              name: postgres-secrets
//        image: quay.io/fedora/postgresql-15:latest
//        imagePullPolicy: IfNotPresent
//        securityContext:
//          runAsNonRoot: true
//          allowPrivilegeEscalation: false
//          seccompProfile:
//            type: RuntimeDefault
//          capabilities:
//            drop:
//            - ALL
//        livenessProbe:
//          exec:
//            command:
//            - /bin/sh
//            - -c
//            - exec pg_isready -U ${POSTGRES_USER} -h 127.0.0.1 -p 5432
//          failureThreshold: 6
//          initialDelaySeconds: 30
//          periodSeconds: 10
//          successThreshold: 1
//          timeoutSeconds: 5
//        name: postgresql
//        ports:
//        - containerPort: 5432
//          name: tcp-postgresql
//          protocol: TCP
//        readinessProbe:
//          exec:
//            command:
//            - /bin/sh
//            - -c
//            - -e
//            - |
//              exec pg_isready -U ${POSTGRES_USER} -h 127.0.0.1 -p 5432
//          failureThreshold: 6
//          initialDelaySeconds: 5
//          periodSeconds: 10
//          successThreshold: 1
//          timeoutSeconds: 5
//        resources:
//          requests:
//            cpu: 250m
//            memory: 256Mi
//          limits:
//            memory: 1024Mi
//        volumeMounts:
//        - mountPath: /dev/shm
//          name: dshm
//        - mountPath: /var/lib/pgsql/data
//          name: data
//      restartPolicy: Always
//      securityContext: {}
//      serviceAccount: default
//      serviceAccountName: default
//      volumes:
//      - emptyDir:
//          medium: Memory
//        name: dshm
//  updateStrategy:
//    rollingUpdate:
//      partition: 0
//    type: RollingUpdate
//  volumeClaimTemplates:
//  - apiVersion: v1
//    kind: PersistentVolumeClaim
//    metadata:
//      name: data
//    spec:
//      accessModes:
//      - ReadWriteOnce
//      resources:
//        requests:
//          storage: 1Gi
//`
//)

const (
	ownerRefFmt = "failed to set owner reference: %s"
)

func (r *BackstageReconciler) reconcileLocalDbStatefulSet(ctx context.Context, backstage bs.Backstage, ns string) error {
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getDefaultDbObjName(backstage),
			Namespace: ns,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, statefulSet, r.localDBStatefulSetMutFun(ctx, statefulSet, backstage, ns)); err != nil {
		if errors.IsConflict(err) {
			return fmt.Errorf("retry sync needed: %v", err)
		}
		return err
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
	visitContainers(&statefulSet.Spec.Template, func(container *v1.Container) {
		if len(container.Image) == 0 || container.Image == fmt.Sprintf("<%s>", bs.EnvPostGresImage) {
			container.Image = r.PsqlImage
		}
	})
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
		return err
	}

	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getDefaultDbObjName(backstage),
			Namespace: backstage.Namespace,
		},
	}
	if _, err := r.cleanupResource(ctx, service, backstage); err != nil {
		return err
	}

	serviceHL := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("backstage-psql-%s-hl", backstage.Name),
			Namespace: backstage.Namespace,
		},
	}
	if _, err := r.cleanupResource(ctx, serviceHL, backstage); err != nil {
		return err
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getDefaultPsqlSecretName(&backstage),
			Namespace: backstage.Namespace,
		},
	}
	if _, err := r.cleanupResource(ctx, secret, backstage); err != nil {
		return err
	}
	return nil
}
