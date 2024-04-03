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
	"os"
	"testing"

	"k8s.io/utils/ptr"

	bsv1alpha1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

var deploymentTestBackstage = bsv1alpha1.Backstage{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "bs",
		Namespace: "ns123",
	},
	Spec: bsv1alpha1.BackstageSpec{
		Database: &bsv1alpha1.Database{
			EnableLocalDb: ptr.To(false),
		},
		Application: &bsv1alpha1.Application{},
	},
}

func TestSpecs(t *testing.T) {
	bs := *deploymentTestBackstage.DeepCopy()
	bs.Spec.Application.Image = ptr.To("my-image:1.0.0")
	bs.Spec.Application.Replicas = ptr.To(int32(3))
	bs.Spec.Application.ImagePullSecrets = []string{"my-secret"}

	testObj := createBackstageTest(bs).withDefaultConfig(true).
		addToDefaultConfig("deployment.yaml", "janus-deployment.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, true, testObj.scheme)
	assert.NoError(t, err)

	assert.Equal(t, "my-image:1.0.0", model.backstageDeployment.container().Image)
	assert.Equal(t, int32(3), *model.backstageDeployment.deployment.Spec.Replicas)
	assert.Equal(t, 1, len(model.backstageDeployment.deployment.Spec.Template.Spec.ImagePullSecrets))
	assert.Equal(t, "my-secret", model.backstageDeployment.deployment.Spec.Template.Spec.ImagePullSecrets[0].Name)

}

// It tests the overriding image feature
// [GA] if we need this (and like this) feature
// we need to think about simple template engine
// for substitution env vars instead.
// Janus image specific
func TestOverrideBackstageImage(t *testing.T) {

	bs := *deploymentTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).
		addToDefaultConfig("deployment.yaml", "janus-deployment.yaml")

	_ = os.Setenv(BackstageImageEnvVar, "dummy")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, false, testObj.scheme)
	assert.NoError(t, err)

	assert.Equal(t, "dummy", model.backstageDeployment.container().Image)

}
