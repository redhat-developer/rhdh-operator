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
	"path/filepath"
	bsv1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha2"
	"redhat-developer/red-hat-developer-hub-operator/pkg/model/multiobject"
	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type BackstagePvcsFactory struct{}

func (f BackstagePvcsFactory) newBackstageObject() RuntimeObject {
	return &BackstagePvcs{}
}

func init() {
	registerConfig("pvcs.yaml", BackstagePvcsFactory{}, true)
}

type BackstagePvcs struct {
	pvcs *multiobject.MultiObject
}

func PvcsName(backstageName, originalName string) string {
	return fmt.Sprintf("%s-%s", utils.GenerateRuntimeObjectName(backstageName, "backstage"), originalName)
}

func (b *BackstagePvcs) Object() runtime.Object {
	return b.pvcs
}

func (b *BackstagePvcs) setObject(object runtime.Object) {
	b.pvcs = object.(*multiobject.MultiObject)
}

func (b *BackstagePvcs) EmptyObject() client.Object {
	return &corev1.PersistentVolumeClaim{}
}

func (b *BackstagePvcs) addToModel(model *BackstageModel, backstage bsv1.Backstage) (bool, error) {
	if b.pvcs != nil {
		model.setRuntimeObject(b)
		return true, nil
	}
	return false, nil
}

func (b *BackstagePvcs) validate(model *BackstageModel, backstage bsv1.Backstage) error {
	for _, o := range b.pvcs.Items {
		_, ok := o.(*corev1.PersistentVolumeClaim)
		if !ok {
			return fmt.Errorf("payload is not corev1.PersistentVolumeClaim: %T", o)
		}
	}
	return nil
}

func (b *BackstagePvcs) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	for _, item := range b.pvcs.Items {
		pvc := item.(*corev1.PersistentVolumeClaim)
		pvc.Name = PvcsName(backstage.Name, pvc.Name)
		setMetaInfo(pvc, backstage, scheme)
	}
}

// implementation of BackstagePodContributor interface
func (b *BackstagePvcs) updatePod(deployment *appsv1.Deployment) {

	for _, pvc := range b.pvcs.Items {
		volName := utils.ToRFC1123Label(pvc.GetName())
		volSrc := corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvc.GetName(),
			},
		}
		deployment.Spec.Template.Spec.Volumes =
			append(deployment.Spec.Template.Spec.Volumes, corev1.Volume{Name: volName, VolumeSource: volSrc})

		c := &deployment.Spec.Template.Spec.Containers[0]

		c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
			Name: volName,
			// TODO make mountDir flexible?
			MountPath: filepath.Join(DefaultMountDir, volName),
		})
	}
}
