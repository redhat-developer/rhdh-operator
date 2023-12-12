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

func (r *BackstageReconciler) extraFilesToVolumes(backstage bs.Backstage) (result []v1.Volume) {
	if backstage.Spec.Application == nil || backstage.Spec.Application.ExtraFiles == nil {
		return nil
	}
	for _, cmExtraFile := range backstage.Spec.Application.ExtraFiles.ConfigMaps {
		result = append(result,
			v1.Volume{
				Name: cmExtraFile.Name,
				VolumeSource: v1.VolumeSource{
					ConfigMap: &v1.ConfigMapVolumeSource{
						DefaultMode:          pointer.Int32(420),
						LocalObjectReference: v1.LocalObjectReference{Name: cmExtraFile.Name},
					},
				},
			},
		)
	}
	for _, secExtraFile := range backstage.Spec.Application.ExtraFiles.Secrets {
		result = append(result,
			v1.Volume{
				Name: secExtraFile.Name,
				VolumeSource: v1.VolumeSource{
					Secret: &v1.SecretVolumeSource{
						DefaultMode: pointer.Int32(420),
						SecretName:  secExtraFile.Name,
					},
				},
			},
		)
	}

	return result
}

func (r *BackstageReconciler) addExtraFilesVolumeMounts(ctx context.Context, backstage bs.Backstage, ns string, deployment *appsv1.Deployment) error {
	if backstage.Spec.Application == nil || backstage.Spec.Application.ExtraFiles == nil {
		return nil
	}

	appConfigFilenamesList, err := r.extractExtraFileNames(ctx, backstage, ns)
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
							MountPath: fmt.Sprintf("%s/%s", backstage.Spec.Application.ExtraFiles.MountPath, f),
							SubPath:   f,
						})
				}
			}
			break
		}
	}
	return nil
}

// extractExtraFileNames returns a mapping of extra-config object name and the list of files in it.
// We intentionally do not return a Map, to preserve the iteration order of the ExtraConfigs in the Custom Resource,
// even though we can't guarantee the iteration order of the files listed inside each ConfigMap or Secret.
func (r *BackstageReconciler) extractExtraFileNames(ctx context.Context, backstage bs.Backstage, ns string) (result []appConfigData, err error) {
	if backstage.Spec.Application == nil || backstage.Spec.Application.ExtraFiles == nil {
		return nil, nil
	}

	for _, cmExtraFile := range backstage.Spec.Application.ExtraFiles.ConfigMaps {
		var files []string
		if cmExtraFile.Key != "" {
			// Limit to that file only
			files = append(files, cmExtraFile.Key)
		} else {
			cm := v1.ConfigMap{}
			if err = r.Get(ctx, types.NamespacedName{Name: cmExtraFile.Name, Namespace: ns}, &cm); err != nil {
				return nil, err
			}
			for filename := range cm.Data {
				// Bear in mind that iteration order over this map is not guaranteed by Go
				files = append(files, filename)
			}
			for filename := range cm.BinaryData {
				// Bear in mind that iteration order over this map is not guaranteed by Go
				files = append(files, filename)
			}
		}
		result = append(result, appConfigData{
			ref:   cmExtraFile.Name,
			files: files,
		})
	}

	for _, secExtraFile := range backstage.Spec.Application.ExtraFiles.Secrets {
		var files []string
		if secExtraFile.Key != "" {
			// Limit to that file only
			files = append(files, secExtraFile.Key)
		} else {
			sec := v1.Secret{}
			if err = r.Get(ctx, types.NamespacedName{Name: secExtraFile.Name, Namespace: ns}, &sec); err != nil {
				return nil, err
			}
			for filename := range sec.Data {
				// Bear in mind that iteration order over this map is not guaranteed by Go
				files = append(files, filename)
			}
		}
		result = append(result, appConfigData{
			ref:   secExtraFile.Name,
			files: files,
		})
	}
	return result, nil
}
