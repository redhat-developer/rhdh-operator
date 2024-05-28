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

	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	bsv1alpha1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

var (
	//secretFilesTestSecret = corev1.Secret{
	//	ObjectMeta: metav1.ObjectMeta{
	//		Name:      "secret1",
	//		Namespace: "ns123",
	//	},
	//	StringData: map[string]string{"conf.yaml": ""},
	//}
	//
	//secretFilesTestSecret2 = corev1.Secret{
	//	ObjectMeta: metav1.ObjectMeta{
	//		Name:      "secret2",
	//		Namespace: "ns123",
	//	},
	//	StringData: map[string]string{"conf2.yaml": ""},
	//}

	secretFilesTestBackstage = bsv1alpha1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bs",
			Namespace: "ns123",
		},
		Spec: bsv1alpha1.BackstageSpec{
			Application: &bsv1alpha1.Application{
				ExtraFiles: &bsv1alpha1.ExtraFiles{
					MountPath: "/my/path",
					Secrets:   []bsv1alpha1.ObjectKeyRef{},
				},
			},
		},
	}
)

func TestDefaultSecretFiles(t *testing.T) {

	bs := *secretFilesTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("secret-files.yaml", "raw-secret-files.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, false, testObj.scheme)

	assert.NoError(t, err)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 1, len(deployment.deployment.Spec.Template.Spec.Containers[0].VolumeMounts))
	assert.Equal(t, 1, len(deployment.deployment.Spec.Template.Spec.Volumes))

}

func TestSpecifiedSecretFiles(t *testing.T) {

	bs := *secretFilesTestBackstage.DeepCopy()
	sf := &bs.Spec.Application.ExtraFiles.Secrets
	*sf = append(*sf, bsv1alpha1.ObjectKeyRef{Name: "secret1", Key: "conf.yaml"})
	*sf = append(*sf, bsv1alpha1.ObjectKeyRef{Name: "secret2", Key: "conf.yaml"})
	// https://issues.redhat.com/browse/RHIDP-2246 - mounting secret/CM with dot in the name
	*sf = append(*sf, bsv1alpha1.ObjectKeyRef{Name: "secret.dot", Key: "conf3.yaml"})

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, false, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 3, len(deployment.deployment.Spec.Template.Spec.Containers[0].VolumeMounts))
	assert.Equal(t, 0, len(deployment.deployment.Spec.Template.Spec.Containers[0].Args))
	assert.Equal(t, 3, len(deployment.deployment.Spec.Template.Spec.Volumes))

	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("secret1"), deployment.podSpec().Volumes[0].Name)
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("secret2"), deployment.podSpec().Volumes[1].Name)
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("secret.dot"), deployment.podSpec().Volumes[2].Name)

}

func TestDefaultAndSpecifiedSecretFiles(t *testing.T) {

	bs := *secretFilesTestBackstage.DeepCopy()
	sf := &bs.Spec.Application.ExtraFiles.Secrets
	*sf = append(*sf, bsv1alpha1.ObjectKeyRef{Name: "secret1", Key: "conf.yaml"})
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("secret-files.yaml", "raw-secret-files.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, true, false, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 2, len(deployment.deployment.Spec.Template.Spec.Containers[0].VolumeMounts))
	assert.Equal(t, 0, len(deployment.deployment.Spec.Template.Spec.Containers[0].Args))
	assert.Equal(t, 2, len(deployment.deployment.Spec.Template.Spec.Volumes))
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("secret1"), deployment.podSpec().Volumes[1].Name)

}
