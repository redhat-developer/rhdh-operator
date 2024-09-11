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
	"k8s.io/apimachinery/pkg/runtime"

	bsv1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha2"
	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
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

func addConfigMapFiles(spec bsv1.BackstageSpec, deployment *appsv1.Deployment, model *BackstageModel) {

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
func (p *ConfigMapFiles) Object() runtime.Object {
	return p.ConfigMap
}

func (p *ConfigMapFiles) setObject(obj runtime.Object) {
	p.ConfigMap = nil
	if obj != nil {
		p.ConfigMap = obj.(*corev1.ConfigMap)
	}

}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) EmptyObject() runtime.Object {
	return &corev1.ConfigMap{}
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	if p.ConfigMap != nil {
		model.setRuntimeObject(p)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) validate(_ *BackstageModel, _ bsv1.Backstage) error {
	return nil
}

func (p *ConfigMapFiles) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	p.ConfigMap.SetName(utils.GenerateRuntimeObjectName(backstage.Name, "backstage-files"))
	setMetaInfo(p.ConfigMap, backstage, scheme)
}

// implementation of BackstagePodContributor interface
func (p *ConfigMapFiles) updatePod(deployment *appsv1.Deployment) {

	utils.MountFilesFrom(&deployment.Spec.Template.Spec, &deployment.Spec.Template.Spec.Containers[0], utils.ConfigMapObjectKind,
		p.ConfigMap.Name, p.MountPath, p.Key, p.ConfigMap.Data)

}
