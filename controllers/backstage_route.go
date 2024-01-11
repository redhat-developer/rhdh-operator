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

	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openshift "github.com/openshift/api/route/v1"
	bs "janus-idp.io/backstage-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *BackstageReconciler) reconcileBackstageRoute(ctx context.Context, backstage *bs.Backstage, ns string) error {
	// Override the route and service names
	name := getDefaultObjName(*backstage)
	route := &openshift.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}

	if !shouldCreateRoute(*backstage) {
		deleted, err := r.cleanupResource(ctx, route, *backstage)
		if err == nil && deleted {
			setStatusCondition(backstage, bs.RouteSynced, metav1.ConditionTrue, bs.Deleted, "")
		}
		return err
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, route, r.routeObjectMutFun(ctx, route, *backstage, ns)); err != nil {
		if errors.IsConflict(err) {
			return fmt.Errorf("retry sync needed: %v", err)
		}
		setStatusCondition(backstage, bs.RouteSynced, metav1.ConditionFalse, bs.SyncFailed, fmt.Sprintf("Error:%s", err.Error()))
		return err
	}
	setStatusCondition(backstage, bs.RouteSynced, metav1.ConditionTrue, bs.SyncOK, fmt.Sprintf("Route host:%s", route.Spec.Host))
	return nil
}

func (r *BackstageReconciler) routeObjectMutFun(ctx context.Context, targetRoute *openshift.Route, backstage bs.Backstage, ns string) controllerutil.MutateFn {
	return func() error {
		route := &openshift.Route{}
		targetRoute.ObjectMeta.DeepCopyInto(&route.ObjectMeta)

		err := r.readConfigMapOrDefault(ctx, backstage.Spec.RawRuntimeConfig.BackstageConfigName, "route.yaml", ns, route)
		if err != nil {
			return err
		}

		// Override the route and service names
		name := getDefaultObjName(backstage)
		route.Name = name
		route.Spec.To.Name = route.Name

		r.labels(&route.ObjectMeta, backstage)

		r.applyRouteParamsFromCR(route, backstage)

		if r.OwnsRuntime {
			if err := controllerutil.SetControllerReference(&backstage, route, r.Scheme); err != nil {
				return fmt.Errorf("failed to set owner reference: %s", err)
			}
		}

		route.ObjectMeta.DeepCopyInto(&targetRoute.ObjectMeta)
		route.Spec.DeepCopyInto(&targetRoute.Spec)
		return nil
	}
}

func (r *BackstageReconciler) applyRouteParamsFromCR(route *openshift.Route, backstage bs.Backstage) {
	if backstage.Spec.Application == nil || backstage.Spec.Application.Route == nil {
		return // Nothing to override
	}
	routeCfg := backstage.Spec.Application.Route
	if len(routeCfg.Host) > 0 {
		route.Spec.Host = routeCfg.Host
	}
	if len(routeCfg.Subdomain) > 0 {
		route.Spec.Subdomain = routeCfg.Subdomain
	}
	if routeCfg.TLS == nil {
		return
	}
	if route.Spec.TLS == nil {
		route.Spec.TLS = &openshift.TLSConfig{
			Termination:                   openshift.TLSTerminationEdge,
			InsecureEdgeTerminationPolicy: openshift.InsecureEdgeTerminationPolicyRedirect,
			Certificate:                   routeCfg.TLS.Certificate,
			Key:                           routeCfg.TLS.Key,
			CACertificate:                 routeCfg.TLS.CACertificate,
			ExternalCertificate: &openshift.LocalObjectReference{
				Name: routeCfg.TLS.ExternalCertificateSecretName,
			},
		}
		return
	}
	if len(routeCfg.TLS.Certificate) > 0 {
		route.Spec.TLS.Certificate = routeCfg.TLS.Certificate
	}
	if len(routeCfg.TLS.Key) > 0 {
		route.Spec.TLS.Key = routeCfg.TLS.Key
	}
	if len(routeCfg.TLS.Certificate) > 0 {
		route.Spec.TLS.Certificate = routeCfg.TLS.Certificate
	}
	if len(routeCfg.TLS.CACertificate) > 0 {
		route.Spec.TLS.CACertificate = routeCfg.TLS.CACertificate
	}
	if len(routeCfg.TLS.ExternalCertificateSecretName) > 0 {
		route.Spec.TLS.ExternalCertificate = &openshift.LocalObjectReference{
			Name: routeCfg.TLS.ExternalCertificateSecretName,
		}
	}
}

func shouldCreateRoute(backstage bs.Backstage) bool {
	if backstage.Spec.Application == nil {
		return true
	}
	if backstage.Spec.Application.Route == nil {
		return true
	}
	return pointer.BoolDeref(backstage.Spec.Application.Route.Enabled, true)
}
