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

package model

import (
	appsv1 "k8s.io/api/apps/v1"

	"redhat-developer/red-hat-developer-hub-operator/api/v1alpha1"
	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ConfigMapFilesFactory struct{}

func (f ConfigMapFilesFactory) newBackstageObject() RuntimeObject {
	return &ConfigMapFiles{MountPath: defaultMountDir}
}

type ConfigMapFiles struct {
	ConfigMap *corev1.ConfigMap
	MountPath string
	Key       string
}

func init() {
	registerConfig("configmap-files.yaml", ConfigMapFilesFactory{})
}

func addConfigMapFiles(spec v1alpha1.BackstageSpec, deployment *appsv1.Deployment, model *BackstageModel) {

	if spec.Application == nil || spec.Application.ExtraFiles == nil || spec.Application.ExtraFiles.ConfigMaps == nil {
		return
	}
	mp := defaultMountDir
	if spec.Application.ExtraFiles.MountPath != "" {
		mp = spec.Application.ExtraFiles.MountPath
	}

	for _, configMap := range spec.Application.ExtraFiles.ConfigMaps {
		cm := model.ExternalConfig.ExtraFileConfigMaps[configMap.Name]
		cmf := ConfigMapFiles{
			ConfigMap: &cm,
			MountPath: mp,
			Key:       configMap.Key,
		}
		cmf.updatePod(deployment)
	}
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) Object() client.Object {
	return p.ConfigMap
}

func (p *ConfigMapFiles) setObject(obj client.Object, backstageName string) {
	p.ConfigMap = nil
	if obj != nil {
		p.ConfigMap = obj.(*corev1.ConfigMap)
	}

}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) EmptyObject() client.Object {
	return &corev1.ConfigMap{}
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) addToModel(model *BackstageModel, backstageMeta v1alpha1.Backstage, ownsRuntime bool) (bool, error) {
	if p.ConfigMap != nil {
		model.setRuntimeObject(p)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) validate(model *BackstageModel, backstage v1alpha1.Backstage) error {
	return nil
}

func (p *ConfigMapFiles) setMetaInfo(backstageName string) {
	p.ConfigMap.SetName(utils.GenerateRuntimeObjectName(backstageName, "default-configmapfiles"))
}

// implementation of BackstagePodContributor interface
func (p *ConfigMapFiles) updatePod(deployment *appsv1.Deployment) {

	utils.MountFilesFrom(&deployment.Spec.Template.Spec, &deployment.Spec.Template.Spec.Containers[0], utils.ConfigMapObjectKind,
		p.ConfigMap.Name, p.MountPath, p.Key, p.ConfigMap.Data)

}
