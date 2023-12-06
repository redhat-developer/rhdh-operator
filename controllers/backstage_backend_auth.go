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
	//	defaultBackstageBackendAuthSecret = `
	//apiVersion: v1
	//kind: Secret
	//metadata:
	//  name: # placeholder for '<cr-name>-auth'
	//data:
	//  # A random value will be generated for the backend-secret key
	//`
)

func (r *BackstageReconciler) handleBackendAuthSecret(ctx context.Context, backstage bs.Backstage, ns string) (secretName string, err error) {
	if backstage.Spec.Application != nil && backstage.Spec.Application.BackendAuthSecretKeyRef != nil {
		return backstage.Spec.Application.BackendAuthSecretKeyRef.Name, nil
	}

	//Create default Secret for backend auth
	var sec v1.Secret
	//var isDefault bool
	err = r.readConfigMapOrDefault(ctx, backstage.Spec.RawRuntimeConfig.BackstageConfigName, "backend-auth-secret.yaml", ns, &sec)
	if err != nil {
		return "", fmt.Errorf("failed to read config: %s", err)
	}
	//Generate a secret if it does not exist
	backendAuthSecretName := fmt.Sprintf("%s-auth", backstage.Name)
	sec.SetName(backendAuthSecretName)
	err = r.Get(ctx, types.NamespacedName{Name: backendAuthSecretName, Namespace: ns}, &sec)
	if err != nil {
		if !errors.IsNotFound(err) {
			return "", fmt.Errorf("failed to get secret for backend auth (%q), reason: %s", backendAuthSecretName, err)
		}
		// there should not be any difference between default and not default
		//		if isDefault {
		// Create a secret with a random value
		authVal := func(length int) string {
			bytes := make([]byte, length)
			if _, randErr := rand.Read(bytes); randErr != nil {
				// Do not fail, but use a fallback value
				return _defaultBackendAuthSecretValue
			}
			return base64.StdEncoding.EncodeToString(bytes)
		}(24)
		sec.Data = map[string][]byte{
			"backend-secret": []byte(authVal),
		}
		//		}
		err = r.Create(ctx, &sec)
		if err != nil {
			return "", fmt.Errorf("failed to create secret for backend auth, reason: %s", err)
		}
	}
	return backendAuthSecretName, nil
}

func (r *BackstageReconciler) addBackendAuthEnvVar(ctx context.Context, backstage bs.Backstage, ns string, deployment *appsv1.Deployment) error {
	backendAuthSecretName, err := r.handleBackendAuthSecret(ctx, backstage, ns)
	if err != nil {
		return err
	}

	if backendAuthSecretName == "" {
		return nil
	}

	for i, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name == _defaultBackstageMainContainerName {
			var k string
			if backstage.Spec.Application != nil && backstage.Spec.Application.BackendAuthSecretKeyRef != nil {
				k = backstage.Spec.Application.BackendAuthSecretKeyRef.Key
			}
			if k == "" {
				//TODO(rm3l): why kubebuilder default values do not work
				k = "backend-secret"
			}
			deployment.Spec.Template.Spec.Containers[i].Env = append(deployment.Spec.Template.Spec.Containers[i].Env,
				v1.EnvVar{
					Name: "BACKEND_SECRET",
					ValueFrom: &v1.EnvVarSource{
						SecretKeyRef: &v1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{
								Name: backendAuthSecretName,
							},
							Key:      k,
							Optional: pointer.Bool(false),
						},
					},
				},
				v1.EnvVar{
					Name:  "APP_CONFIG_backend_auth_keys",
					Value: `[{"secret": "$(BACKEND_SECRET)"}]`,
				})
			break
		}
	}
	return nil
}
