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

	bs "backstage.io/backstage-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
)

//var (
//	defaultDynamicPluginsConfigMap = `
//apiVersion: v1
//kind: ConfigMap
//metadata:
//  name: # placeholder for '<cr-name>-dynamic-plugins'
//data:
//  "dynamic-plugins.yaml": |
//    includes:
//    - dynamic-plugins.default.yaml
//    plugins: []
//`
//)

func (r *BackstageReconciler) getOrGenerateDynamicPluginsConf(ctx context.Context, backstage bs.Backstage, ns string) (config bs.DynamicPluginsConfigRef, err error) {
	if backstage.Spec.DynamicPluginsConfig != nil {
		return *backstage.Spec.DynamicPluginsConfig, nil
	}

	//Create default ConfigMap for dynamic plugins
	var cm v1.ConfigMap
	err = r.readConfigMapOrDefault(ctx, backstage.Spec.RawRuntimeConfig.BackstageConfigName, "dynamic-plugins-configmap.yaml", ns, &cm)
	if err != nil {
		return bs.DynamicPluginsConfigRef{}, fmt.Errorf("failed to read config: %s", err)
	}

	dpConfigName := fmt.Sprintf("%s-dynamic-plugins", backstage.Name)
	cm.SetName(dpConfigName)
	err = r.Get(ctx, types.NamespacedName{Name: dpConfigName, Namespace: ns}, &cm)
	if err != nil {
		if !errors.IsNotFound(err) {
			return bs.DynamicPluginsConfigRef{}, fmt.Errorf("failed to get config map for dynamic plugins (%q), reason: %s", dpConfigName, err)
		}
		err = r.Create(ctx, &cm)
		if err != nil {
			return bs.DynamicPluginsConfigRef{}, fmt.Errorf("failed to create config map for dynamic plugins, reason: %s", err)
		}
	}

	return bs.DynamicPluginsConfigRef{
		Name: dpConfigName,
		Kind: "ConfigMap",
	}, nil
}

func (r *BackstageReconciler) getDynamicPluginsConfVolume(ctx context.Context, backstage bs.Backstage, ns string) (*v1.Volume, error) {
	dpConf, err := r.getOrGenerateDynamicPluginsConf(ctx, backstage, ns)
	if err != nil {
		return nil, err
	}

	if dpConf.Name == "" {
		return nil, nil
	}

	var volumeSource v1.VolumeSource
	switch dpConf.Kind {
	case "ConfigMap":
		volumeSource.ConfigMap = &v1.ConfigMapVolumeSource{
			DefaultMode:          pointer.Int32(420),
			LocalObjectReference: v1.LocalObjectReference{Name: dpConf.Name},
		}
	case "Secret":
		volumeSource.Secret = &v1.SecretVolumeSource{
			DefaultMode: pointer.Int32(420),
			SecretName:  dpConf.Name,
		}
	}

	return &v1.Volume{
		Name:         dpConf.Name,
		VolumeSource: volumeSource,
	}, nil
}

func (r *BackstageReconciler) addDynamicPluginsConfVolumeMount(ctx context.Context, backstage bs.Backstage, ns string, deployment *appsv1.Deployment) error {
	dpConf, err := r.getOrGenerateDynamicPluginsConf(ctx, backstage, ns)
	if err != nil {
		return err
	}

	if dpConf.Name == "" {
		return nil
	}

	for i, c := range deployment.Spec.Template.Spec.InitContainers {
		if c.Name == _defaultBackstageInitContainerName {
			deployment.Spec.Template.Spec.InitContainers[i].VolumeMounts = append(deployment.Spec.Template.Spec.InitContainers[i].VolumeMounts,
				v1.VolumeMount{
					Name:      dpConf.Name,
					MountPath: fmt.Sprintf("%s/dynamic-plugins.yaml", _containersWorkingDir),
					ReadOnly:  true,
					SubPath:   "dynamic-plugins.yaml",
				})
			break
		}
	}
	return nil
}
