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
	"redhat-developer/red-hat-developer-hub-operator/api/v1alpha2"
	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ConfigMapEnvsFactory struct{}

func (f ConfigMapEnvsFactory) newBackstageObject() RuntimeObject {
	return &ConfigMapEnvs{}
}

type ConfigMapEnvs struct {
	ConfigMap *corev1.ConfigMap
	Key       string
}

func init() {
	registerConfig("configmap-envs.yaml", ConfigMapEnvsFactory{})
}

func addConfigMapEnvs(spec v1alpha2.BackstageSpec, deployment *appsv1.Deployment, model *BackstageModel) {

	if spec.Application == nil || spec.Application.ExtraEnvs == nil || spec.Application.ExtraEnvs.ConfigMaps == nil {
		return
	}

	for _, configMap := range spec.Application.ExtraEnvs.ConfigMaps {
		cm := model.ExternalConfig.ExtraEnvConfigMaps[configMap.Name]
		cmf := ConfigMapEnvs{
			ConfigMap: &cm,
			Key:       configMap.Key,
		}
		cmf.updatePod(deployment)
	}
}

// Object implements RuntimeObject interface
func (p *ConfigMapEnvs) Object() client.Object {
	return p.ConfigMap
}

func (p *ConfigMapEnvs) setObject(obj client.Object) {
	p.ConfigMap = nil
	if obj != nil {
		p.ConfigMap = obj.(*corev1.ConfigMap)
	}
}

// EmptyObject implements RuntimeObject interface
func (p *ConfigMapEnvs) EmptyObject() client.Object {
	return &corev1.ConfigMap{}
}

// implementation of RuntimeObject interface
func (p *ConfigMapEnvs) addToModel(model *BackstageModel, _ v1alpha2.Backstage) (bool, error) {
	if p.ConfigMap != nil {
		model.setRuntimeObject(p)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapEnvs) validate(_ *BackstageModel, _ v1alpha2.Backstage) error {
	return nil
}

func (p *ConfigMapEnvs) setMetaInfo(backstageName string) {
	p.ConfigMap.SetName(utils.GenerateRuntimeObjectName(backstageName, "backstage-envs"))
}

// implementation of BackstagePodContributor interface
func (p *ConfigMapEnvs) updatePod(deployment *appsv1.Deployment) {

	utils.AddEnvVarsFrom(&deployment.Spec.Template.Spec.Containers[0], utils.ConfigMapObjectKind,
		p.ConfigMap.Name, p.Key)
}
