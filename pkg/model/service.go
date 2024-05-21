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
	"fmt"

	bsv1alpha1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha1"
	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BackstageServiceFactory struct{}

func (f BackstageServiceFactory) newBackstageObject() RuntimeObject {
	return &BackstageService{}
}

type BackstageService struct {
	service *corev1.Service
}

func init() {
	registerConfig("service.yaml", BackstageServiceFactory{})
}

func ServiceName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage")
}

// implementation of RuntimeObject interface
func (b *BackstageService) Object() client.Object {
	return b.service
}

func (b *BackstageService) setObject(obj client.Object) {
	b.service = nil
	if obj != nil {
		b.service = obj.(*corev1.Service)
	}
}

// implementation of RuntimeObject interface
func (b *BackstageService) addToModel(model *BackstageModel, _ bsv1alpha1.Backstage) (bool, error) {
	if b.service == nil {
		return false, fmt.Errorf("Backstage Service is not initialized, make sure there is service.yaml in default or raw configuration")
	}
	model.backstageService = b
	model.setRuntimeObject(b)

	return true, nil

}

// implementation of RuntimeObject interface
func (b *BackstageService) EmptyObject() client.Object {
	return &corev1.Service{}
}

// implementation of RuntimeObject interface
func (b *BackstageService) validate(_ *BackstageModel, _ bsv1alpha1.Backstage) error {
	return nil
}

func (b *BackstageService) setMetaInfo(backstageName string) {
	b.service.SetName(ServiceName(backstageName))
	utils.GenerateLabel(&b.service.Spec.Selector, BackstageAppLabel, utils.BackstageAppLabelValue(backstageName))
}
