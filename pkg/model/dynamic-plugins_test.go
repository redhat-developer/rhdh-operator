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
	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"
	"testing"

	"k8s.io/utils/ptr"

	bsv1alpha1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

var testDynamicPluginsBackstage = bsv1alpha1.Backstage{
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

func TestDynamicPluginsValidationFailed(t *testing.T) {

	bs := testDynamicPluginsBackstage.DeepCopy()

	testObj := createBackstageTest(*bs).withDefaultConfig(true).
		addToDefaultConfig("dynamic-plugins.yaml", "raw-dynamic-plugins.yaml")

	_, err := InitObjects(context.TODO(), *bs, testObj.externalConfig, true, false, testObj.scheme)

	//"failed object validation, reason: failed to find initContainer named install-dynamic-plugins")
	assert.Error(t, err)

}

// Janus pecific test
func TestDefaultDynamicPlugins(t *testing.T) {

	bs := testDynamicPluginsBackstage.DeepCopy()

	testObj := createBackstageTest(*bs).withDefaultConfig(true).
		addToDefaultConfig("dynamic-plugins.yaml", "raw-dynamic-plugins.yaml").
		addToDefaultConfig("deployment.yaml", "janus-deployment.yaml")

	model, err := InitObjects(context.TODO(), *bs, testObj.externalConfig, true, false, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model.backstageDeployment)
	//dynamic-plugins-root
	//dynamic-plugins-npmrc
	//vol-default-dynamic-plugins
	assert.Equal(t, 3, len(model.backstageDeployment.deployment.Spec.Template.Spec.Volumes))

	ic := initContainer(model)
	assert.NotNil(t, ic)
	//dynamic-plugins-root
	//dynamic-plugins-npmrc
	//vol-default-dynamic-plugins
	assert.Equal(t, 3, len(ic.VolumeMounts))

}

func TestDefaultAndSpecifiedDynamicPlugins(t *testing.T) {

	bs := testDynamicPluginsBackstage.DeepCopy()
	bs.Spec.Application.DynamicPluginsConfigMapName = "dplugin"

	testObj := createBackstageTest(*bs).withDefaultConfig(true).
		addToDefaultConfig("dynamic-plugins.yaml", "raw-dynamic-plugins.yaml").
		addToDefaultConfig("deployment.yaml", "janus-deployment.yaml")

	testObj.externalConfig.DynamicPlugins = corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "dplugin"}}

	model, err := InitObjects(context.TODO(), *bs, testObj.externalConfig, true, false, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	ic := initContainer(model)
	assert.NotNil(t, ic)
	//dynamic-plugins-root
	//dynamic-plugins-npmrc
	//vol-dplugin
	assert.Equal(t, 3, len(ic.VolumeMounts))
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("dplugin"), ic.VolumeMounts[2].Name)
	//t.Log(">>>>>>>>>>>>>>>>", ic.VolumeMounts)
}

func TestDynamicPluginsFailOnArbitraryDepl(t *testing.T) {

	bs := testDynamicPluginsBackstage.DeepCopy()
	//bs.Spec.Application.DynamicPluginsConfigMapName = "dplugin"

	testObj := createBackstageTest(*bs).withDefaultConfig(true).
		addToDefaultConfig("dynamic-plugins.yaml", "raw-dynamic-plugins.yaml")

	_, err := InitObjects(context.TODO(), *bs, testObj.externalConfig, true, false, testObj.scheme)

	assert.Error(t, err)
}

func initContainer(model *BackstageModel) *corev1.Container {
	for _, v := range model.backstageDeployment.deployment.Spec.Template.Spec.InitContainers {
		if v.Name == dynamicPluginInitContainerName {
			return &v
		}
	}
	return nil
}
