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
	bsv1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha2"
	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AuditLogPvcFactory struct{}

func (f AuditLogPvcFactory) newBackstageObject() RuntimeObject {
	return &AuditLogPvc{}
}

type AuditLogPvc struct {
	pvc *corev1.PersistentVolumeClaim
}

func init() {
	registerConfig("audit-log-pvc.yaml", AuditLogPvcFactory{})
}

func AuditLogPvcDefaultName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "audit-log")
}

// implementation of RuntimeObject interface
func (b *AuditLogPvc) Object() client.Object {
	return b.pvc
}

// implementation of RuntimeObject interface
func (b *AuditLogPvc) setObject(obj client.Object) {
	b.pvc = nil
	if obj != nil {
		b.pvc = obj.(*corev1.PersistentVolumeClaim)
	}
}

// implementation of RuntimeObject interface
func (b *AuditLogPvc) addToModel(model *BackstageModel, backstage bsv1.Backstage) (bool, error) {

	if b.pvc != nil {
		model.setRuntimeObject(b)
		model.localAuditLogPvc = b
		return true, nil
	}

	return false, nil
}

// implementation of RuntimeObject interface
func (b *AuditLogPvc) EmptyObject() client.Object {
	return &corev1.PersistentVolumeClaim{}
}

// implementation of RuntimeObject interface
func (b *AuditLogPvc) validate(model *BackstageModel, backstage bsv1.Backstage) error {
	b.pvc.Name = AuditLogPvcDefaultName(backstage.Name)
	return nil
}

func (b *AuditLogPvc) setMetaInfo(backstageName string) {
	b.pvc.SetName(AuditLogPvcDefaultName(backstageName))
}
