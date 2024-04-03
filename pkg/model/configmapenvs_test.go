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
	"context"
	"testing"

	"k8s.io/utils/ptr"

	corev1 "k8s.io/api/core/v1"

	bsv1alpha1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfigMapEnvFrom(t *testing.T) {

	bs := bsv1alpha1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bs",
			Namespace: "ns123",
		},
		Spec: bsv1alpha1.BackstageSpec{
			Database: &bsv1alpha1.Database{
				EnableLocalDb: ptr.To(false),
			},
		},
	}

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("configmap-envs.yaml", "raw-cm-envs.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, false, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	bscontainer := model.backstageDeployment.deployment.Spec.Template.Spec.Containers[0]
	assert.NotNil(t, bscontainer)

	assert.Equal(t, 1, len(bscontainer.EnvFrom))
	assert.Equal(t, 0, len(bscontainer.Env))

}

func TestSpecifiedConfigMapEnvs(t *testing.T) {

	bs := bsv1alpha1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bs",
			Namespace: "ns123",
		},
		Spec: bsv1alpha1.BackstageSpec{
			Application: &bsv1alpha1.Application{
				ExtraEnvs: &bsv1alpha1.ExtraEnvs{
					ConfigMaps: []bsv1alpha1.ObjectKeyRef{},
				},
			},
		},
	}

	bs.Spec.Application.ExtraEnvs.ConfigMaps = append(bs.Spec.Application.ExtraEnvs.ConfigMaps,
		bsv1alpha1.ObjectKeyRef{Name: "mapName", Key: "ENV1"})

	testObj := createBackstageTest(bs).withDefaultConfig(true)
	testObj.externalConfig.ExtraEnvConfigMaps["mapName"] = corev1.ConfigMap{Data: map[string]string{"mapName": "ENV1"}}

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, false, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	bscontainer := model.backstageDeployment.container()
	assert.NotNil(t, bscontainer)
	assert.Equal(t, 1, len(bscontainer.Env))

	assert.NotNil(t, bscontainer.Env[0])
	assert.Equal(t, "ENV1", bscontainer.Env[0].ValueFrom.ConfigMapKeyRef.Key)

}
