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
	"crypto/rand"
	"encoding/base64"
	"fmt"

	bs "janus-idp.io/backstage-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
)

var (
	_defaultBackendAuthSecretValue = "pl4s3Ch4ng3M3"
	_backendSecretKey              = "backend-secret"
)

func (r *BackstageReconciler) getBackendAuthAppConfig(
	ctx context.Context,
	backstage bs.Backstage,
	ns string,
) (backendAuthAppConfig *bs.ObjectKeyRef, err error) {
	if backstage.Spec.Application != nil && backstage.Spec.Application.AppConfig != nil {
		// Users are expected to provide their own app-configs with the right backend auth secret
		return nil, nil
	}

	var cm v1.ConfigMap
	err = r.readConfigMapOrDefault(ctx, backstage.Spec.RawRuntimeConfig.BackstageConfigName, "backend-auth-configmap.yaml", ns, &cm)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %s", err)
	}
	// Create ConfigMap
	backendAuthCmName := fmt.Sprintf("%s-auth", backstage.Name)
	cm.SetName(backendAuthCmName)
	err = r.Get(ctx, types.NamespacedName{Name: backendAuthCmName, Namespace: ns}, &cm)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get ConfigMap for backend auth (%q), reason: %s", backendAuthCmName, err)
		}
		err = r.Create(ctx, &cm)
		if err != nil {
			return nil, fmt.Errorf("failed to create ConfigMap for backend auth, reason: %s", err)
		}
	}

	var sec v1.Secret
	err = r.readConfigMapOrDefault(ctx, backstage.Spec.RawRuntimeConfig.BackstageConfigName, "backend-auth-secret.yaml", ns, &sec)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %s", err)
	}
	sec.SetName(backendAuthCmName)
	err = r.Get(ctx, types.NamespacedName{Name: backendAuthCmName, Namespace: ns}, &sec)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get Secret for backend auth (%q), reason: %s", backendAuthCmName, err)
		}
		authVal := func(length int) string {
			bytes := make([]byte, length)
			if _, randErr := rand.Read(bytes); randErr != nil {
				// Do not fail, but use a fallback value
				return _defaultBackendAuthSecretValue
			}
			return base64.StdEncoding.EncodeToString(bytes)
		}(24)
		sec.StringData = map[string]string{
			_backendSecretKey: authVal,
		}
		err = r.Create(ctx, &sec)
		if err != nil {
			return nil, fmt.Errorf("failed to create Secret for backend auth, reason: %s", err)
		}
	}

	return &bs.ObjectKeyRef{Name: backendAuthCmName}, nil
}

func (r *BackstageReconciler) addBackendAuthEnvVar(ctx context.Context, backstage bs.Backstage, ns string, deployment *appsv1.Deployment) error {
	backendAuthAppConfig, err := r.getBackendAuthAppConfig(ctx, backstage, ns)
	if err != nil {
		return err
	}
	if backendAuthAppConfig == nil {
		return nil
	}

	hasEnvVar := func(c v1.Container, name string) bool {
		for _, envVar := range c.Env {
			if envVar.Name == name {
				return true
			}
		}
		return false
	}

	for i, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name == _defaultBackstageMainContainerName {
			if !hasEnvVar(c, "BACKEND_SECRET") {
				deployment.Spec.Template.Spec.Containers[i].Env = append(deployment.Spec.Template.Spec.Containers[i].Env,
					v1.EnvVar{
						Name: "BACKEND_SECRET",
						ValueFrom: &v1.EnvVarSource{
							SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									// Secret has the same name as the backend auth ConfigMap
									Name: backendAuthAppConfig.Name,
								},
								Key:      _backendSecretKey,
								Optional: pointer.Bool(false),
							},
						},
					})
			}
			break
		}
	}

	return nil
}
