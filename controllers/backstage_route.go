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
	"k8s.io/apimachinery/pkg/types"
)

func (r *BackstageReconciler) applyBackstageRoute(ctx context.Context, backstage bs.Backstage, ns string) error {
	if !shouldCreateRoute(backstage) {
		return nil
	}

	route := &openshift.Route{}
	err := r.readConfigMapOrDefault(ctx, backstage.Spec.RawRuntimeConfig.BackstageConfigName, "route.yaml", ns, route)
	if err != nil {
		return err
	}

	// Override the route and service names
	name := fmt.Sprintf("backstage-%s", backstage.Name)
	route.Name = name
	route.Spec.To.Name = name

	err = r.Get(ctx, types.NamespacedName{Name: route.Name, Namespace: ns}, route)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get backstage route, reason: %s", err)
		}
	} else {
		//lg.Info("CR update is ignored for the time")
		return nil
	}

	r.labels(&route.ObjectMeta, backstage)

	if r.OwnsRuntime {
		if err := controllerutil.SetControllerReference(&backstage, route, r.Scheme); err != nil {
			return fmt.Errorf("failed to set owner reference: %s", err)
		}
	}

	r.applyRouteParamsFromCR(route, backstage)

	err = r.Create(ctx, route)
	if err != nil {
		return fmt.Errorf("failed to create backstage route, reason: %s", err)
	}
	return nil
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
