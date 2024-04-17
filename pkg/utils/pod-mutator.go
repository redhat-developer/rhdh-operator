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

package utils

import (
	"path/filepath"

	"k8s.io/utils/ptr"

	corev1 "k8s.io/api/core/v1"
)

const (
	SecretObjectKind    = "Secret"
	ConfigMapObjectKind = "ConfigMap"
)

type ObjectKind string

type PodMutator struct {
	PodSpec   *corev1.PodSpec
	Container *corev1.Container
}

// MountFilesFrom adds Volume to specified podSpec and related VolumeMounts to specified belonging to this podSpec container
// from ConfigMap or Secret volume source
// podSpec - PodSpec to add Volume to
// container - container to add VolumeMount(s) to
// kind - kind of source, can be ConfigMap or Secret
// object name - name of source object
// mountPath - mount path, default one or  as it specified in BackstageCR.spec.Application.AppConfig|ExtraFiles
// fileName - file name which fits one of the object's key, otherwise error will be returned.
// data - key:value pairs from the object. should be specified if fileName specified
func MountFilesFrom(podSpec *corev1.PodSpec, container *corev1.Container, kind ObjectKind, objectName, mountPath, fileName string, data map[string]string) {

	volName := GenerateVolumeNameFromCmOrSecret(objectName)
	volSrc := corev1.VolumeSource{}
	if kind == ConfigMapObjectKind {
		volSrc.ConfigMap = &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: objectName},
			DefaultMode:          ptr.To(int32(420)),
			Optional:             ptr.To(false),
		}
	} else if kind == SecretObjectKind {
		volSrc.Secret = &corev1.SecretVolumeSource{
			SecretName:  objectName,
			DefaultMode: ptr.To(int32(420)),
			Optional:    ptr.To(false),
		}
	}

	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{Name: volName, VolumeSource: volSrc})

	if data != nil {
		for file := range data {
			if fileName == "" || fileName == file {
				vm := corev1.VolumeMount{Name: volName, MountPath: filepath.Join(mountPath, file), SubPath: file, ReadOnly: true}
				container.VolumeMounts = append(container.VolumeMounts, vm)
			}
		}
	} else {
		vm := corev1.VolumeMount{Name: volName, MountPath: filepath.Join(mountPath, objectName), ReadOnly: true}
		container.VolumeMounts = append(container.VolumeMounts, vm)
	}

}

func AddEnvVarsFrom(container *corev1.Container, kind ObjectKind, objectName, varName string) {

	if varName == "" {
		envFromSrc := corev1.EnvFromSource{}
		if kind == ConfigMapObjectKind {
			envFromSrc.ConfigMapRef = &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: objectName}}
		} else if kind == SecretObjectKind {
			envFromSrc.SecretRef = &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: objectName}}
		}
		container.EnvFrom = append(container.EnvFrom, envFromSrc)
	} else {
		envVarSrc := &corev1.EnvVarSource{}
		if kind == ConfigMapObjectKind {
			envVarSrc.ConfigMapKeyRef = &corev1.ConfigMapKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: objectName,
				},
				Key: varName,
			}
		} else if kind == SecretObjectKind {
			envVarSrc.SecretKeyRef = &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: objectName,
				},
				Key: varName,
			}
		}
		container.Env = append(container.Env, corev1.EnvVar{
			Name:      varName,
			ValueFrom: envVarSrc,
		})
	}
}

func SetDbSecretEnvVar(container *corev1.Container, secretName string) {
	AddEnvVarsFrom(container, SecretObjectKind, secretName, "")
}

// sets pullSecret for Pod
func SetImagePullSecrets(podSpec *corev1.PodSpec, pullSecrets []string) {
	if pullSecrets == nil {
		return
	}
	podSpec.ImagePullSecrets = []corev1.LocalObjectReference{}
	for _, ps := range pullSecrets {
		podSpec.ImagePullSecrets = append(podSpec.ImagePullSecrets, corev1.LocalObjectReference{Name: ps})
	}
}
