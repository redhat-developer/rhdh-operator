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
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
)

type appConfigData struct {
	ref   string
	files []string
}

func (r *BackstageReconciler) appConfigsToVolumes(backstage bs.Backstage) (result []v1.Volume) {
	if backstage.Spec.Application == nil || backstage.Spec.Application.AppConfig == nil {
		return nil
	}
	for _, cmRef := range backstage.Spec.Application.AppConfig.ConfigMapRefs {
		volumeSource := v1.VolumeSource{
			ConfigMap: &v1.ConfigMapVolumeSource{
				DefaultMode:          pointer.Int32(420),
				LocalObjectReference: v1.LocalObjectReference{Name: cmRef},
			},
		}
		result = append(result,
			v1.Volume{
				Name:         cmRef,
				VolumeSource: volumeSource,
			},
		)
	}

	return result
}

func (r *BackstageReconciler) addAppConfigsVolumeMounts(ctx context.Context, backstage bs.Backstage, ns string, deployment *appsv1.Deployment) error {
	if backstage.Spec.Application == nil || backstage.Spec.Application.AppConfig == nil {
		return nil
	}

	appConfigFilenamesList, err := r.extractAppConfigFileNames(ctx, backstage, ns)
	if err != nil {
		return err
	}

	for i, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name == _defaultBackstageMainContainerName {
			for _, appConfigFilenames := range appConfigFilenamesList {
				for _, f := range appConfigFilenames.files {
					deployment.Spec.Template.Spec.Containers[i].VolumeMounts = append(deployment.Spec.Template.Spec.Containers[i].VolumeMounts,
						v1.VolumeMount{
							Name:      appConfigFilenames.ref,
							MountPath: fmt.Sprintf("%s/%s", backstage.Spec.Application.AppConfig.MountPath, f),
							SubPath:   f,
						})
				}
			}
			break
		}
	}
	return nil
}

func (r *BackstageReconciler) addAppConfigsContainerArgs(ctx context.Context, backstage bs.Backstage, ns string, deployment *appsv1.Deployment) error {
	if backstage.Spec.Application == nil || backstage.Spec.Application.AppConfig == nil {
		return nil
	}

	appConfigFilenamesList, err := r.extractAppConfigFileNames(ctx, backstage, ns)
	if err != nil {
		return err
	}

	for i, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name == _defaultBackstageMainContainerName {
			for _, appConfigFilenames := range appConfigFilenamesList {
				// Args
				for _, fileName := range appConfigFilenames.files {
					appConfigPath := fmt.Sprintf("%s/%s", backstage.Spec.Application.AppConfig.MountPath, fileName)
					deployment.Spec.Template.Spec.Containers[i].Args =
						append(deployment.Spec.Template.Spec.Containers[i].Args, "--config", appConfigPath)
				}
			}
			break
		}
	}
	return nil
}

// extractAppConfigFileNames returns a mapping of app-config object name and the list of files in it.
// We intentionally do not return a Map, to preserve the iteration order of the AppConfigs in the Custom Resource,
// even though we can't guarantee the iteration order of the files listed inside each ConfigMap or Secret.
func (r *BackstageReconciler) extractAppConfigFileNames(ctx context.Context, backstage bs.Backstage, ns string) (result []appConfigData, err error) {
	if backstage.Spec.Application == nil || backstage.Spec.Application.AppConfig == nil {
		return nil, nil
	}

	for _, cmRef := range backstage.Spec.Application.AppConfig.ConfigMapRefs {
		cm := v1.ConfigMap{}
		if err = r.Get(ctx, types.NamespacedName{Name: cmRef, Namespace: ns}, &cm); err != nil {
			return nil, err
		}
		var files []string
		for filename := range cm.Data {
			// Bear in mind that iteration order over this map is not guaranteed by Go
			files = append(files, filename)
		}
		for filename := range cm.BinaryData {
			// Bear in mind that iteration order over this map is not guaranteed by Go
			files = append(files, filename)
		}
		result = append(result, appConfigData{
			ref:   cmRef,
			files: files,
		})
	}
	return result, nil
}
