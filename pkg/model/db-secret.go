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
	"strconv"

	"k8s.io/apimachinery/pkg/runtime"

	bsv1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha2"
	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type DbSecretFactory struct{}

func (f DbSecretFactory) newBackstageObject() RuntimeObject {
	return &DbSecret{}
}

type DbSecret struct {
	secret *corev1.Secret
}

func init() {
	registerConfig("db-secret.yaml", DbSecretFactory{})
}

func DbSecretDefaultName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-psql-secret")
}

// implementation of RuntimeObject interface
func (b *DbSecret) Object() runtime.Object {
	return b.secret
}

// implementation of RuntimeObject interface
func (b *DbSecret) setObject(obj runtime.Object) {
	b.secret = nil
	if obj != nil {
		b.secret = obj.(*corev1.Secret)
	}
}

// implementation of RuntimeObject interface
func (b *DbSecret) addToModel(model *BackstageModel, backstage bsv1.Backstage) (bool, error) {

	// do not add if specified
	if backstage.Spec.IsAuthSecretSpecified() {
		return false, nil
	}

	if b.secret != nil && model.localDbEnabled {
		model.setRuntimeObject(b)
		model.LocalDbSecret = b
		return true, nil
	}

	return false, nil
}

// implementation of RuntimeObject interface
func (b *DbSecret) EmptyObject() runtime.Object {
	return &corev1.Secret{}
}

// implementation of RuntimeObject interface
func (b *DbSecret) validate(model *BackstageModel, backstage bsv1.Backstage) error {

	pswd, _ := utils.GeneratePassword(24)

	service := model.LocalDbService

	b.secret.StringData = map[string]string{
		"POSTGRES_PASSWORD":         pswd,
		"POSTGRESQL_ADMIN_PASSWORD": pswd,
		"POSTGRES_USER":             "postgres",
		"POSTGRES_HOST":             service.service.GetName(),
		"POSTGRES_PORT":             strconv.FormatInt(int64(service.service.Spec.Ports[0].Port), 10),
	}

	return nil
}

func (b *DbSecret) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	b.secret.SetName(DbSecretDefaultName(backstage.Name))
	setMetaInfo(b.secret, backstage, scheme)
}
