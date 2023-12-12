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

	bs "janus-idp.io/backstage-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *BackstageReconciler) getBackendAuthAppConfig(
	ctx context.Context,
	backstage bs.Backstage,
	ns string,
) (backendAuthAppConfig *bs.ObjectKeyRef, err error) {
	if backstage.Spec.Application != nil &&
		(backstage.Spec.Application.AppConfig != nil || backstage.Spec.Application.ExtraFiles != nil || backstage.Spec.Application.ExtraEnvs != nil) {
		// Users are expected to fill their app-config(s) with their own backend auth key
		return nil, nil
	}

	var cm v1.ConfigMap
	err = r.readConfigMapOrDefault(ctx, backstage.Spec.RawRuntimeConfig.BackstageConfigName, "backend-auth-configmap.yaml", ns, &cm)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %s", err)
	}
	// Create ConfigMap
	backendAuthCmName := fmt.Sprintf("%s-auth-app-config", backstage.Name)
	cm.SetName(backendAuthCmName)
	err = r.Get(ctx, types.NamespacedName{Name: backendAuthCmName, Namespace: ns}, &cm)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get ConfigMap for backend auth (%q), reason: %s", backendAuthCmName, err)
		}
		setBackstageAppLabel(&cm.ObjectMeta.Labels, backstage)
		r.labels(&cm.ObjectMeta, backstage)

		if r.OwnsRuntime {
			if err = controllerutil.SetControllerReference(&backstage, &cm, r.Scheme); err != nil {
				return nil, fmt.Errorf("failed to set owner reference: %s", err)
			}
		}
		err = r.Create(ctx, &cm)
		if err != nil {
			return nil, fmt.Errorf("failed to create ConfigMap for backend auth, reason: %s", err)
		}
	}

	return &bs.ObjectKeyRef{Name: backendAuthCmName}, nil
}
