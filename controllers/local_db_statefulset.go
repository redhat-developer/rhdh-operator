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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

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
//	DefaultLocalDbService = `apiVersion: v1
//kind: Service
//metadata:
//  name: backstage-psql-cr1 # placeholder for 'backstage-psql-<cr-name>'
//spec:
//  selector:
//      janus-idp.io/app:  backstage-psql-cr1 # placeholder for 'backstage-psql-<cr-name>'
//  ports:
//    - port: 5432
//`
//	DefaultLocalDbServiceHL = `apiVersion: v1
//kind: Service
//metadata:
//  name: backstage-psql-cr1-hl # placeholder for 'backstage-psql-<cr-name>-hl'
//spec:
//  selector:
//      janus-idp.io/app:  backstage-psql-cr1 # placeholder for 'backstage-psql-<cr-name>'
//  clusterIP: None
//  ports:
//    - port: 5432
//`
//)

const ownerRefFmt = "failed to set owner reference: %s"

func (r *BackstageReconciler) applyLocalDbStatefulSet(ctx context.Context, backstage bs.Backstage, ns string) error {

	lg := log.FromContext(ctx)

	statefulSet := &appsv1.StatefulSet{}
	err := r.readConfigMapOrDefault(ctx, backstage.Spec.RawRuntimeConfig.LocalDbConfigName, "db-statefulset.yaml", ns, statefulSet)
	if err != nil {
		return err
	}

	// need to patch the Name before get for correct search
	statefulSet.Name = fmt.Sprintf("backstage-psql-%s", backstage.Name)
	err = r.Get(ctx, types.NamespacedName{Name: statefulSet.Name, Namespace: ns}, statefulSet)
	if err != nil {
		if errors.IsNotFound(err) {

		} else {
			return fmt.Errorf(ownerRefFmt, err)
		}
	} else {
		lg.Info("CR update is ignored for the time")
		return nil
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

	r.labels(&statefulSet.ObjectMeta, backstage)
	if err = r.patchLocalDbStatefulSetObj(statefulSet, backstage); err != nil {
		return err
	}

	err = r.Create(ctx, statefulSet)
	if err != nil {
		return fmt.Errorf("failed to create statefulset, reason: %s", err)
	}

	return nil
}

func (r *BackstageReconciler) applyLocalDbServices(ctx context.Context, backstage bs.Backstage, ns string) error {
	// TODO static for the time and bound to Secret: postgres-secret
	label := fmt.Sprintf("backstage-psql-%s", backstage.Name)
	err := r.applyPsqlService(ctx, backstage, "backstage-psql", label, ns, "db-service.yaml")
	if err != nil {
		return err
	}
	nameHL := fmt.Sprintf("backstage-psql-%s-hl", backstage.Name)
	return r.applyPsqlService(ctx, backstage, nameHL, label, ns, "db-service-hl.yaml")

}

func (r *BackstageReconciler) applyPsqlService(ctx context.Context, backstage bs.Backstage, name, label, ns string, key string) error {

	lg := log.FromContext(ctx)

	service := &corev1.Service{}
	err := r.readConfigMapOrDefault(ctx, backstage.Spec.RawRuntimeConfig.LocalDbConfigName, key, ns, service)
	if err != nil {
		return err
	}
	service.SetName(name)
	setBackstageLocalDbLabel(&service.ObjectMeta.Labels, label)
	setBackstageLocalDbLabel(&service.Spec.Selector, label)
	err = r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, service)
	if err != nil {
		if errors.IsNotFound(err) {

		} else {
			return fmt.Errorf("failed to get service, reason: %s", err)
		}
	} else {
		lg.Info("CR update is ignored for the time")
		return nil
	}

	if r.OwnsRuntime {
		if err := controllerutil.SetControllerReference(&backstage, service, r.Scheme); err != nil {
			return fmt.Errorf(ownerRefFmt, err)
		}
	}

	err = r.Create(ctx, service)
	if err != nil {
		return fmt.Errorf("failed to create service, reason: %s", err)
	}

	return nil
}

func (r *BackstageReconciler) patchLocalDbStatefulSetObj(statefulSet *appsv1.StatefulSet, backstage bs.Backstage) error {
	name := fmt.Sprintf("backstage-psql-%s", backstage.Name)
	statefulSet.SetName(name)
	statefulSet.Spec.Template.SetName(name)
	statefulSet.Spec.ServiceName = fmt.Sprintf("%s-hl", name)

	setBackstageLocalDbLabel(&statefulSet.Spec.Template.ObjectMeta.Labels, name)
	setBackstageLocalDbLabel(&statefulSet.Spec.Selector.MatchLabels, name)

	return nil
}
