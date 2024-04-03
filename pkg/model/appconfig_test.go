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

	bsv1alpha1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"

	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	appConfigTestCm = corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-config1",
			Namespace: "ns123",
		},
		Data: map[string]string{"conf.yaml": "conf.yaml data"},
	}

	appConfigTestCm2 = corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-config2",
			Namespace: "ns123",
		},
		Data: map[string]string{"conf21.yaml": "", "conf22.yaml": ""},
	}

	appConfigTestCm3 = corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-config3",
			Namespace: "ns123",
		},
		Data: map[string]string{"conf31.yaml": "", "conf32.yaml": ""},
	}

	appConfigTestBackstage = bsv1alpha1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bs",
			Namespace: "ns123",
		},
		Spec: bsv1alpha1.BackstageSpec{
			Application: &bsv1alpha1.Application{
				AppConfig: &bsv1alpha1.AppConfig{
					MountPath:  "/my/path",
					ConfigMaps: []bsv1alpha1.ObjectKeyRef{},
				},
			},
		},
	}
)

func TestDefaultAppConfig(t *testing.T) {

	//bs := simpleTestBackstage()
	bs := *appConfigTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("app-config.yaml", "raw-app-config.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, false, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 1, len(deployment.deployment.Spec.Template.Spec.Containers[0].VolumeMounts))
	assert.Contains(t, deployment.deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath, defaultMountDir)
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret(AppConfigDefaultName(bs.Name)), deployment.deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name)
	assert.Equal(t, 2, len(deployment.deployment.Spec.Template.Spec.Containers[0].Args))
	assert.Equal(t, 1, len(deployment.deployment.Spec.Template.Spec.Volumes))

}

func TestSpecifiedAppConfig(t *testing.T) {

	bs := *appConfigTestBackstage.DeepCopy()
	bs.Spec.Application.AppConfig.ConfigMaps = append(bs.Spec.Application.AppConfig.ConfigMaps,
		bsv1alpha1.ObjectKeyRef{Name: appConfigTestCm.Name})
	bs.Spec.Application.AppConfig.ConfigMaps = append(bs.Spec.Application.AppConfig.ConfigMaps,
		bsv1alpha1.ObjectKeyRef{Name: appConfigTestCm2.Name})
	bs.Spec.Application.AppConfig.ConfigMaps = append(bs.Spec.Application.AppConfig.ConfigMaps,
		bsv1alpha1.ObjectKeyRef{Name: appConfigTestCm3.Name, Key: "conf31.yaml"})

	testObj := createBackstageTest(bs).withDefaultConfig(true)
	testObj.externalConfig.AppConfigs = map[string]corev1.ConfigMap{appConfigTestCm.Name: appConfigTestCm, appConfigTestCm2.Name: appConfigTestCm2,
		appConfigTestCm3.Name: appConfigTestCm3}
	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig,
		true, false, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 4, len(deployment.container().VolumeMounts))
	assert.Contains(t, deployment.container().VolumeMounts[0].MountPath,
		bs.Spec.Application.AppConfig.MountPath)
	assert.Equal(t, 8, len(deployment.container().Args))
	assert.Equal(t, 3, len(deployment.deployment.Spec.Template.Spec.Volumes))

}

func TestDefaultAndSpecifiedAppConfig(t *testing.T) {

	bs := *appConfigTestBackstage.DeepCopy()
	cms := &bs.Spec.Application.AppConfig.ConfigMaps
	*cms = append(*cms, bsv1alpha1.ObjectKeyRef{Name: appConfigTestCm.Name})

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("app-config.yaml", "raw-app-config.yaml")

	//testObj.detailedSpec.AddConfigObject(&AppConfig{ConfigMap: &cm, MountPath: "/my/path"})
	testObj.externalConfig.AppConfigs[appConfigTestCm.Name] = appConfigTestCm

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, false, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 2, len(deployment.deployment.Spec.Template.Spec.Containers[0].VolumeMounts))
	assert.Equal(t, 4, len(deployment.deployment.Spec.Template.Spec.Containers[0].Args))
	assert.Equal(t, 2, len(deployment.deployment.Spec.Template.Spec.Volumes))

	//assert.Equal(t, filepath.Dir(deployment.deployment.Spec.Template.Spec.Containers[0].Args[1]),
	//	deployment.deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath)

	// it should be valid assertion using Volumes and VolumeMounts indexes since the order of adding is from default to specified

	//assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret()deployment.deployment.Spec.Template.Spec.Volumes[0].Name
	assert.Equal(t, deployment.deployment.Spec.Template.Spec.Volumes[0].Name,
		deployment.deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name)

	//t.Log(">>>>>>>>>>>>>>>>", )
	//t.Log(">>>>>>>>>>>>>>>>", )

}
