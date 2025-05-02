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
	"path/filepath"
	"testing"

	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"
	"github.com/redhat-developer/rhdh-operator/pkg/platform"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDefaultPvcs(t *testing.T) {
	__sealights__.StartTestFunc("639cbfa5923235fd8a", t)
	defer func() { __sealights__.EndTestFunc("639cbfa5923235fd8a", t) }()

	bs := bsv1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pvc",
		},
	}

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("pvcs.yaml", "multi-pvc.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model)

	obj := model.getRuntimeObjectByType(&BackstagePvcs{})
	assert.NotNil(t, obj)
	assert.Equal(t, utils.GetObjectKind(&corev1.PersistentVolumeClaim{}, testObj.scheme).Kind, obj.Object().GetObjectKind().GroupVersionKind().Kind)
	mv, ok := obj.Object().(*multiobject.MultiObject)
	assert.True(t, ok)
	assert.Equal(t, 2, len(mv.Items))
	assert.Equal(t, PvcsName(bs.Name, "myclaim1"), mv.Items[0].GetName())
	assert.Equal(t, "myclaim1", mv.Items[0].GetAnnotations()[ConfiguredNameAnnotation])
	assert.Equal(t, "/mount/path/from/annotation", mv.Items[1].GetAnnotations()[DefaultMountPathAnnotation])

	// PVC volumes created and mounted to backstage container
	assert.Equal(t, 2, len(model.backstageDeployment.podSpec().Volumes))
	assert.Equal(t, PvcsName(bs.Name, "myclaim1"), model.backstageDeployment.podSpec().Volumes[0].Name)
	assert.Equal(t, 2, len(model.backstageDeployment.container().VolumeMounts))
	assert.Equal(t, PvcsName(bs.Name, "myclaim1"), model.backstageDeployment.container().VolumeMounts[0].Name)
	assert.Equal(t, filepath.Join(DefaultMountDir, PvcsName(bs.Name, "myclaim1")), model.backstageDeployment.container().VolumeMounts[0].MountPath)
	assert.Equal(t, "/mount/path/from/annotation", model.backstageDeployment.container().VolumeMounts[1].MountPath)

}

func TestMultiContainersPvc(t *testing.T) {
	__sealights__.StartTestFunc("c68d229fe92146b397", t)
	defer func() { __sealights__.EndTestFunc("c68d229fe92146b397", t) }()
	bs := bsv1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pvc",
		},
	}

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("deployment.yaml", "multicontainer-deployment.yaml").addToDefaultConfig("pvcs.yaml", "multi-pvc-containers.yaml")
	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model)
	assert.Equal(t, 4, len(model.backstageDeployment.allContainers()))

	assert.Equal(t, 3, len(model.backstageDeployment.podSpec().Volumes))
	// myclaim1(default), myclaim2(listed), myclaim3(*)
	assert.Equal(t, 3, len(model.backstageDeployment.containerByName("backstage-backend").VolumeMounts))
	// myclaim2(listed), myclaim3(*)
	assert.Equal(t, 2, len(model.backstageDeployment.containerByName("install-dynamic-plugins").VolumeMounts))
	// myclaim3(*)
	assert.Equal(t, 1, len(model.backstageDeployment.containerByName("another-container").VolumeMounts))
	// myclaim3(*)
	assert.Equal(t, 1, len(model.backstageDeployment.containerByName("another-init-container").VolumeMounts))

}

func TestSpecifiedPvcs(t *testing.T) {
	__sealights__.StartTestFunc("755940cdf353f225ae", t)
	defer func() { __sealights__.EndTestFunc("755940cdf353f225ae", t) }()
	bs := bsv1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pvc",
		},
		Spec: bsv1.BackstageSpec{
			Application: &bsv1.Application{
				ExtraFiles: &bsv1.ExtraFiles{
					Pvcs: []bsv1.PvcRef{
						{
							Name: "my-pvc1",
						},
						{
							Name:      "my-pvc2",
							MountPath: "/my/pvc/path",
						},
					},
				},
			},
		},
	}

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	testObj.externalConfig.ExtraPvcKeys = []string{"my-pvc1", "my-pvc2"}

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)
	assert.NotNil(t, model)
	d := model.backstageDeployment
	assert.Equal(t, 2, len(d.podSpec().Volumes))
	assert.Equal(t, 2, len(d.container().VolumeMounts))
	assert.Equal(t, "my-pvc1", d.container().VolumeMounts[0].Name)
	assert.Equal(t, filepath.Join(DefaultMountDir, "my-pvc1"), d.container().VolumeMounts[0].MountPath)
	assert.Equal(t, "my-pvc2", d.container().VolumeMounts[1].Name)
	assert.Equal(t, "/my/pvc/path", d.container().VolumeMounts[1].MountPath)
}
