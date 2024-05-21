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
	"os"
	"strconv"

	"k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/client"

	bs "redhat-developer/red-hat-developer-hub-operator/api/v1alpha1"

	"redhat-developer/red-hat-developer-hub-operator/pkg/model"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const AutoSyncEnvVar = "EXT_CONF_SYNC_backstage"

// Add additional details to the Backstage Spec helping in making Backstage RuntimeObjects Model
// Validates Backstage Spec and fails fast if something not correct
func (r *BackstageReconciler) preprocessSpec(ctx context.Context, backstage bs.Backstage) (model.ExternalConfig, error) {
	//lg := log.FromContext(ctx)

	bsSpec := backstage.Spec
	ns := backstage.Namespace

	result := model.NewExternalConfig()

	// Process RawConfig
	if bsSpec.RawRuntimeConfig != nil {
		if bsSpec.RawRuntimeConfig.BackstageConfigName != "" {
			cm := &corev1.ConfigMap{}
			if err := r.addExtConfig(&result, ctx, cm, backstage.Name, bsSpec.RawRuntimeConfig.BackstageConfigName, ns); err != nil {
				return result, err
			}
			for key, value := range cm.Data {
				result.RawConfig[key] = value
			}
		}
		if bsSpec.RawRuntimeConfig.LocalDbConfigName != "" {
			cm := &corev1.ConfigMap{}
			if err := r.addExtConfig(&result, ctx, cm, backstage.Name, bsSpec.RawRuntimeConfig.LocalDbConfigName, ns); err != nil {
				return result, err
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
		for _, ac := range bsSpec.Application.AppConfig.ConfigMaps {
			cm := &corev1.ConfigMap{}
			if err := r.addExtConfig(&result, ctx, cm, backstage.Name, ac.Name, ns); err != nil {
				return result, err
			}
			result.AppConfigs[ac.Name] = *cm
		}
	}

	// Process ConfigMapFiles
	if bsSpec.Application.ExtraFiles != nil && bsSpec.Application.ExtraFiles.ConfigMaps != nil {
		for _, ef := range bsSpec.Application.ExtraFiles.ConfigMaps {
			cm := &corev1.ConfigMap{}
			if err := r.addExtConfig(&result, ctx, cm, backstage.Name, ef.Name, ns); err != nil {
				return result, err
			}
			result.ExtraFileConfigMaps[cm.Name] = *cm
		}
	}

	// Process SecretFiles
	if bsSpec.Application.ExtraFiles != nil && bsSpec.Application.ExtraFiles.Secrets != nil {
		for _, ef := range bsSpec.Application.ExtraFiles.Secrets {
			secret := &corev1.Secret{}
			if err := r.addExtConfig(&result, ctx, secret, backstage.Name, ef.Name, ns); err != nil {
				return result, err
			}
			result.ExtraFileSecrets[secret.Name] = *secret
		}
	}

	// Process ConfigMapEnvs
	if bsSpec.Application.ExtraEnvs != nil && bsSpec.Application.ExtraEnvs.ConfigMaps != nil {
		for _, ee := range bsSpec.Application.ExtraEnvs.ConfigMaps {
			cm := &corev1.ConfigMap{}
			if err := r.addExtConfig(&result, ctx, cm, backstage.Name, ee.Name, ns); err != nil {
				return result, err
			}
			result.ExtraEnvConfigMaps[cm.Name] = *cm
		}
	}

	// Process SecretEnvs
	if bsSpec.Application.ExtraEnvs != nil && bsSpec.Application.ExtraEnvs.Secrets != nil {
		for _, ee := range bsSpec.Application.ExtraEnvs.Secrets {
			secret := &corev1.Secret{}
			if err := r.addExtConfig(&result, ctx, secret, backstage.Name, ee.Name, ns); err != nil {
				return result, err
			}
			result.ExtraEnvSecrets[secret.Name] = *secret
		}
	}

	// Process DynamicPlugins
	if bsSpec.Application.DynamicPluginsConfigMapName != "" {
		cm := &corev1.ConfigMap{}
		if err := r.addExtConfig(&result, ctx, cm, backstage.Name, bsSpec.Application.DynamicPluginsConfigMapName, ns); err != nil {
			return result, err
		}
		result.DynamicPlugins = *cm
	}

	return result, nil
}

func (r *BackstageReconciler) addExtConfig(config *model.ExternalConfig, ctx context.Context, obj client.Object, backstageName, objectName, ns string) error {

	lg := log.FromContext(ctx)

	if err := r.Get(ctx, types.NamespacedName{Name: objectName, Namespace: ns}, obj); err != nil {
		if _, ok := obj.(*corev1.Secret); ok && errors.IsForbidden(err) {
			return fmt.Errorf("warning: Secrets GET is forbidden, updating Secrets may not cause Pod recreating")
		}
		return fmt.Errorf("failed to get external config from %s: %s", objectName, err)
	}

	if err := config.AddToSyncedConfig(obj); err != nil {
		return fmt.Errorf("failed to add to synced %s: %s", obj.GetName(), err)
	}

	if obj.GetLabels() == nil {
		obj.SetLabels(map[string]string{})
	}
	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(map[string]string{})
	}

	autoSync := true
	autoSyncStr, ok := os.LookupEnv(AutoSyncEnvVar)
	if ok {
		autoSync, _ = strconv.ParseBool(autoSyncStr)
	}

	if obj.GetLabels()[model.ExtConfigSyncLabel] == "" || obj.GetAnnotations()[model.BackstageNameAnnotation] == "" ||
		obj.GetLabels()[model.ExtConfigSyncLabel] != strconv.FormatBool(autoSync) {

		obj.GetLabels()[model.ExtConfigSyncLabel] = strconv.FormatBool(autoSync)
		obj.GetAnnotations()[model.BackstageNameAnnotation] = backstageName
		if err := r.Update(ctx, obj); err != nil {
			return fmt.Errorf("failed to update external config %s: %s", objectName, err)
		}
		lg.V(1).Info(fmt.Sprintf("update external config %s with label %s=%s and annotation %s=%s", obj.GetName(), model.ExtConfigSyncLabel, strconv.FormatBool(autoSync), model.BackstageNameAnnotation, backstageName))
	}

	return nil
}
