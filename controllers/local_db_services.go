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
func (r *BackstageReconciler) reconcileLocalDbServices(ctx context.Context, backstage *bs.Backstage, ns string) error {
	name := getDefaultDbObjName(*backstage)
	err := r.reconcilePsqlService(ctx, backstage, name, name, "db-service.yaml", ns)
	if err != nil {
		return err
	}
	nameHL := fmt.Sprintf("backstage-psql-%s-hl", backstage.Name)
	return r.reconcilePsqlService(ctx, backstage, nameHL, name, "db-service-hl.yaml", ns)

}

func (r *BackstageReconciler) reconcilePsqlService(ctx context.Context, backstage *bs.Backstage, serviceName, label, configKey, ns string) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: ns,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, r.serviceObjectMutFun(ctx, service, *backstage, backstage.Spec.RawRuntimeConfig.LocalDbConfigName, configKey, serviceName, label)); err != nil {
		if errors.IsConflict(err) {
			return retryReconciliation(err)
		}
		msg := fmt.Sprintf("failed to deploy database service: %s", err)
		setStatusCondition(backstage, bs.ConditionDeployed, metav1.ConditionFalse, bs.DeployFailed, msg)
	}
	return nil
}
