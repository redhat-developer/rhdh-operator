package utils

import (
	corev1 "k8s.io/api/core/v1"
)

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
