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
	"os"

	corev1 "k8s.io/api/core/v1"

	bsv1alpha1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha1"
	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const LocalDbImageEnvVar = "RELATED_IMAGE_postgresql"

type DbStatefulSetFactory struct{}

func (f DbStatefulSetFactory) newBackstageObject() RuntimeObject {
	return &DbStatefulSet{}
}

type DbStatefulSet struct {
	statefulSet *appsv1.StatefulSet
}

func init() {
	registerConfig("db-statefulset.yaml", DbStatefulSetFactory{})
}

func DbStatefulSetName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-db")
}

// implementation of RuntimeObject interface
func (b *DbStatefulSet) Object() client.Object {
	return b.statefulSet
}

func (b *DbStatefulSet) setObject(obj client.Object) {
	b.statefulSet = nil
	if obj != nil {
		b.statefulSet = obj.(*appsv1.StatefulSet)
	}
}

// implementation of RuntimeObject interface
func (b *DbStatefulSet) addToModel(model *BackstageModel, _ bsv1alpha1.Backstage) (bool, error) {
	if b.statefulSet == nil {
		if model.localDbEnabled {
			return false, fmt.Errorf("LocalDb StatefulSet not configured, make sure there is db-statefulset.yaml.yaml in default or raw configuration")
		}
		return false, nil
	} else {
		if !model.localDbEnabled {
			return false, nil
		}
	}

	model.localDbStatefulSet = b
	model.setRuntimeObject(b)

	// override image with env var
	// [GA] Do we really need this feature?
	if os.Getenv(LocalDbImageEnvVar) != "" {
		b.container().Image = os.Getenv(LocalDbImageEnvVar)
	}

	return true, nil
}

// implementation of RuntimeObject interface
func (b *DbStatefulSet) EmptyObject() client.Object {
	return &appsv1.StatefulSet{}
}

// implementation of RuntimeObject interface
func (b *DbStatefulSet) validate(model *BackstageModel, backstage bsv1alpha1.Backstage) error {

	if backstage.Spec.Application != nil {
		utils.SetImagePullSecrets(b.podSpec(), backstage.Spec.Application.ImagePullSecrets)
	}
	if backstage.Spec.IsAuthSecretSpecified() {
		utils.SetDbSecretEnvVar(b.container(), backstage.Spec.Database.AuthSecretName)
	} else if model.LocalDbSecret != nil {
		utils.SetDbSecretEnvVar(b.container(), model.LocalDbSecret.secret.Name)
	}
	return nil
}

func (b *DbStatefulSet) setMetaInfo(backstageName string) {
	b.statefulSet.SetName(DbStatefulSetName(backstageName))
	utils.GenerateLabel(&b.statefulSet.Spec.Template.ObjectMeta.Labels, BackstageAppLabel, fmt.Sprintf("backstage-db-%s", backstageName))
	utils.GenerateLabel(&b.statefulSet.Spec.Selector.MatchLabels, BackstageAppLabel, fmt.Sprintf("backstage-db-%s", backstageName))
}

// returns DB container
func (b *DbStatefulSet) container() *corev1.Container {
	return &b.podSpec().Containers[0]
}

// returns DB pod
func (b *DbStatefulSet) podSpec() *corev1.PodSpec {
	return &b.statefulSet.Spec.Template.Spec
}
