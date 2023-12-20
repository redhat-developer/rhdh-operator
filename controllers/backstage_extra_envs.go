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
	bs "janus-idp.io/backstage-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
)

func (r *BackstageReconciler) addExtraEnvs(backstage bs.Backstage, deployment *appsv1.Deployment) {
	if backstage.Spec.Application == nil || backstage.Spec.Application.ExtraEnvs == nil {
		return
	}

	for _, env := range backstage.Spec.Application.ExtraEnvs.Envs {
		for i := range deployment.Spec.Template.Spec.Containers {
			deployment.Spec.Template.Spec.Containers[i].Env = append(deployment.Spec.Template.Spec.Containers[i].Env, v1.EnvVar{
				Name:  env.Name,
				Value: env.Value,
			})
		}
	}

	for _, cmRef := range backstage.Spec.Application.ExtraEnvs.ConfigMaps {
		for i := range deployment.Spec.Template.Spec.Containers {
			if cmRef.Key != "" {
				deployment.Spec.Template.Spec.Containers[i].Env = append(deployment.Spec.Template.Spec.Containers[i].Env, v1.EnvVar{
					Name: cmRef.Key,
					ValueFrom: &v1.EnvVarSource{
						ConfigMapKeyRef: &v1.ConfigMapKeySelector{
							LocalObjectReference: v1.LocalObjectReference{Name: cmRef.Name},
							Key:                  cmRef.Key,
						},
					},
				})
			} else {
				deployment.Spec.Template.Spec.Containers[i].EnvFrom = append(deployment.Spec.Template.Spec.Containers[i].EnvFrom, v1.EnvFromSource{
					ConfigMapRef: &v1.ConfigMapEnvSource{
						LocalObjectReference: v1.LocalObjectReference{Name: cmRef.Name},
					},
				})
			}
		}
	}

	for _, secRef := range backstage.Spec.Application.ExtraEnvs.Secrets {
		for i := range deployment.Spec.Template.Spec.Containers {
			if secRef.Key != "" {
				deployment.Spec.Template.Spec.Containers[i].Env = append(deployment.Spec.Template.Spec.Containers[i].Env, v1.EnvVar{
					Name: secRef.Key,
					ValueFrom: &v1.EnvVarSource{
						SecretKeyRef: &v1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{Name: secRef.Name},
							Key:                  secRef.Key,
						},
					},
				})
			} else {
				deployment.Spec.Template.Spec.Containers[i].EnvFrom = append(deployment.Spec.Template.Spec.Containers[i].EnvFrom, v1.EnvFromSource{
					SecretRef: &v1.SecretEnvSource{
						LocalObjectReference: v1.LocalObjectReference{Name: secRef.Name},
					},
				})
			}
		}
	}
}
