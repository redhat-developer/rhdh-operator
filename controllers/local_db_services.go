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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	bs "janus-idp.io/backstage-operator/api/v1alpha1"
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
func (r *BackstageReconciler) applyLocalDbServices(ctx context.Context, backstage bs.Backstage, ns string) error {
	name := fmt.Sprintf("backstage-psql-%s", backstage.Name)
	err := r.applyPsqlService(ctx, backstage, name, name, ns, "db-service.yaml")
	if err != nil {
		return err
	}
	nameHL := fmt.Sprintf("backstage-psql-%s-hl", backstage.Name)
	return r.applyPsqlService(ctx, backstage, nameHL, name, ns, "db-service-hl.yaml")

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
