package utils

import (
	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"

	corev1 "k8s.io/api/core/v1"
)

// sets pullSecret for Pod
func SetImagePullSecrets(podSpec *corev1.PodSpec, pullSecrets []string) {
	__sealights__.TraceFunc("9970141d4f00aec2eb")
	if pullSecrets == nil {
		return
	}
	podSpec.ImagePullSecrets = []corev1.LocalObjectReference{}
	for _, ps := range pullSecrets {
		podSpec.ImagePullSecrets = append(podSpec.ImagePullSecrets, corev1.LocalObjectReference{Name: ps})
	}
}
