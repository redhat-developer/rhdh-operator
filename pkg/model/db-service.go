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

	"k8s.io/apimachinery/pkg/runtime"

	bsv1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha2"
	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type DbServiceFactory struct{}

func (f DbServiceFactory) newBackstageObject() RuntimeObject {
	return &DbService{}
}

type DbService struct {
	service *corev1.Service
}

func init() {
	registerConfig("db-service.yaml", DbServiceFactory{})
}

func DbServiceName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-psql")
}

// implementation of RuntimeObject interface
func (b *DbService) Object() runtime.Object {
	return b.service
}

func (b *DbService) setObject(obj runtime.Object) {
	b.service = nil
	if obj != nil {
		b.service = obj.(*corev1.Service)
	}
}

// implementation of RuntimeObject interface
func (b *DbService) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	if b.service == nil {
		if model.localDbEnabled {
			return false, fmt.Errorf("LocalDb Service not initialized, make sure there is db-service.yaml.yaml in default or raw configuration")
		}
		return false, nil
	} else {
		if !model.localDbEnabled {
			return false, nil
		}
	}

	// force this service to be headless even if it is not set in the original config
	b.service.Spec.ClusterIP = corev1.ClusterIPNone

	model.LocalDbService = b
	model.setRuntimeObject(b)

	return true, nil
}

// implementation of RuntimeObject interface
func (b *DbService) EmptyObject() runtime.Object {
	return &corev1.Service{}
}

// implementation of RuntimeObject interface
func (b *DbService) validate(_ *BackstageModel, _ bsv1.Backstage) error {
	return nil
}

func (b *DbService) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	b.service.SetName(DbServiceName(backstage.Name))
	utils.GenerateLabel(&b.service.Spec.Selector, BackstageAppLabel, utils.BackstageDbAppLabelValue(backstage.Name))
	setMetaInfo(b.service, backstage, scheme)
}
