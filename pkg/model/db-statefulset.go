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

	bsv1alpha1 "janus-idp.io/backstage-operator/api/v1alpha1"
	"janus-idp.io/backstage-operator/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const LocalDbImageEnvVar = "RELATED_IMAGE_postgresql"

type DbStatefulSetFactory struct{}

func (f DbStatefulSetFactory) newBackstageObject() RuntimeObject {
	return &DbStatefulSet{ /*statefulSet: &appsv1.StatefulSet{}*/ }
}

type DbStatefulSet struct {
	statefulSet *appsv1.StatefulSet
	secretName  string
}

func init() {
	registerConfig("db-statefulset.yaml", DbStatefulSetFactory{})
}

func DbStatefulSetName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "db-statefulset")
}

// implementation of RuntimeObject interface
func (b *DbStatefulSet) Object() client.Object {
	return b.statefulSet
}

func (b *DbStatefulSet) setObject(obj client.Object, name string) {
	b.statefulSet = nil
	if obj != nil {
		b.statefulSet = obj.(*appsv1.StatefulSet)
	}
}

// implementation of RuntimeObject interface
func (b *DbStatefulSet) addToModel(model *BackstageModel, backstageMeta bsv1alpha1.Backstage, ownsRuntime bool) error {
	if b.statefulSet == nil {
		if model.localDbEnabled {
			return fmt.Errorf("LocalDb StatefulSet not configured, make sure there is db-statefulset.yaml.yaml in default or raw configuration")
		}
		return nil
	} else {
		if !model.localDbEnabled {
			return nil
		}
	}

	model.localDbStatefulSet = b
	model.setRuntimeObject(b)

	b.statefulSet.SetName(utils.GenerateRuntimeObjectName(backstageMeta.Name, "db-statefulset"))
	utils.GenerateLabel(&b.statefulSet.Spec.Template.ObjectMeta.Labels, backstageAppLabel, fmt.Sprintf("backstage-db-%s", backstageMeta.Name))
	utils.GenerateLabel(&b.statefulSet.Spec.Selector.MatchLabels, backstageAppLabel, fmt.Sprintf("backstage-db-%s", backstageMeta.Name))

	// override image with env var
	// [GA] TODO Do we really need this feature?
	if os.Getenv(LocalDbImageEnvVar) != "" {
		b.container().Image = os.Getenv(LocalDbImageEnvVar)
	}

	return nil
}

// implementation of RuntimeObject interface
func (b *DbStatefulSet) EmptyObject() client.Object {
	return &appsv1.StatefulSet{}
}

// implementation of RuntimeObject interface
func (b *DbStatefulSet) validate(model *BackstageModel, backstage bsv1alpha1.Backstage) error {
	return nil
}

// Injects DB Secret name as an env variable of DB container
// Local DB pod considered to have single container
func (b *DbStatefulSet) setSecretNameEnvFrom(envFrom corev1.EnvFromSource) {

	// it is possible that Secret name already set by default configuration
	// has to be overriden in this case
	if b.secretName != "" {
		//var ind int
		for i, v := range b.container().EnvFrom {
			if v.SecretRef.Name == b.secretName {
				b.statefulSet.Spec.Template.Spec.Containers[0].EnvFrom[i] = envFrom
				//ind = i
				break
			}
		}

	} else {
		b.statefulSet.Spec.Template.Spec.Containers[0].EnvFrom = append(b.statefulSet.Spec.Template.Spec.Containers[0].EnvFrom, envFrom)
	}
	b.secretName = envFrom.SecretRef.Name
}

// returns DB container
func (b *DbStatefulSet) container() *corev1.Container {
	return &b.podSpec().Containers[0]
}

// returns DB pod
func (b *DbStatefulSet) podSpec() corev1.PodSpec {
	return b.statefulSet.Spec.Template.Spec
}

func (b *DbStatefulSet) setDbEnvsFromSecret(name string) {
	b.container().EnvFrom = append(b.container().EnvFrom, corev1.EnvFromSource{
		SecretRef: &corev1.SecretEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: name}}})
}
