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

	bs "redhat-developer/red-hat-developer-hub-operator/api/v1alpha1"

	"redhat-developer/red-hat-developer-hub-operator/pkg/model"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Add additional details to the Backstage Spec helping in making Backstage RuntimeObjects Model
// Validates Backstage Spec and fails fast if something not correct
func (r *BackstageReconciler) preprocessSpec(ctx context.Context, backstage bs.Backstage) (model.ExternalConfig, error) {
	//lg := log.FromContext(ctx)

	bsSpec := backstage.Spec
	ns := backstage.Namespace

	result := model.ExternalConfig{
		RawConfig:           map[string]string{},
		AppConfigs:          map[string]corev1.ConfigMap{},
		ExtraFileConfigMaps: map[string]corev1.ConfigMap{},
		ExtraEnvConfigMaps:  map[string]corev1.ConfigMap{},
	}

	// Process RawConfig
	if bsSpec.RawRuntimeConfig != nil {
		if bsSpec.RawRuntimeConfig.BackstageConfigName != "" {
			cm := corev1.ConfigMap{}
			if err := r.Get(ctx, types.NamespacedName{Name: bsSpec.RawRuntimeConfig.BackstageConfigName, Namespace: ns}, &cm); err != nil {
				return result, fmt.Errorf("failed to load rawConfig %s: %w", bsSpec.RawRuntimeConfig.BackstageConfigName, err)
			}
			for key, value := range cm.Data {
				result.RawConfig[key] = value
			}
		}
		if bsSpec.RawRuntimeConfig.LocalDbConfigName != "" {
			cm := corev1.ConfigMap{}
			if err := r.Get(ctx, types.NamespacedName{Name: bsSpec.RawRuntimeConfig.LocalDbConfigName, Namespace: ns}, &cm); err != nil {
				return result, fmt.Errorf("failed to load rawConfig %s: %w", bsSpec.RawRuntimeConfig.LocalDbConfigName, err)
			}
			for key, value := range cm.Data {
				result.RawConfig[key] = value
			}
		}
	}

	if bsSpec.Application == nil {
		bsSpec.Application = &bs.Application{}
	}

	// Process AppConfigs
	if bsSpec.Application.AppConfig != nil {
		//mountPath := bsSpec.Application.AppConfig.MountPath
		for _, ac := range bsSpec.Application.AppConfig.ConfigMaps {
			cm := corev1.ConfigMap{}
			if err := r.Get(ctx, types.NamespacedName{Name: ac.Name, Namespace: ns}, &cm); err != nil {
				return result, fmt.Errorf("failed to get configMap %s: %w", ac.Name, err)
			}
			result.AppConfigs[cm.Name] = cm
		}
	}

	// Process ConfigMapFiles
	if bsSpec.Application.ExtraFiles != nil && bsSpec.Application.ExtraFiles.ConfigMaps != nil {
		for _, ef := range bsSpec.Application.ExtraFiles.ConfigMaps {
			cm := corev1.ConfigMap{}
			if err := r.Get(ctx, types.NamespacedName{Name: ef.Name, Namespace: ns}, &cm); err != nil {
				return result, fmt.Errorf("failed to get ConfigMap %s: %w", ef.Name, err)
			}
			result.ExtraFileConfigMaps[cm.Name] = cm
		}
	}

	// Process ConfigMapEnvs
	if bsSpec.Application.ExtraEnvs != nil && bsSpec.Application.ExtraEnvs.ConfigMaps != nil {
		for _, ee := range bsSpec.Application.ExtraEnvs.ConfigMaps {
			cm := corev1.ConfigMap{}
			if err := r.Get(ctx, types.NamespacedName{Name: ee.Name, Namespace: ns}, &cm); err != nil {
				return result, fmt.Errorf("failed to get configMap %s: %w", ee.Name, err)
			}
			result.ExtraEnvConfigMaps[cm.Name] = cm
		}
	}

	// Process DynamicPlugins
	if bsSpec.Application.DynamicPluginsConfigMapName != "" {
		cm := corev1.ConfigMap{}
		if err := r.Get(ctx, types.NamespacedName{Name: bsSpec.Application.DynamicPluginsConfigMapName,
			Namespace: ns}, &cm); err != nil {
			return result, fmt.Errorf("failed to get ConfigMap %v: %w", cm, err)
		}
		result.DynamicPlugins = cm
	}

	return result, nil
}
