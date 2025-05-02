package controller

import (
	"context"

	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"

	"github.com/redhat-developer/rhdh-operator/pkg/platform"
	ctrl "sigs.k8s.io/controller-runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
)

func DetectPlatform() (platform.Platform, error) {
	__sealights__.TraceFunc("64b761c0d0a7338d8b")

	config := ctrl.GetConfigOrDie()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return platform.Default, err
	}

	discoveryClient := discovery.NewDiscoveryClient(clientset.RESTClient())

	// Check for OpenShift
	apiGroups, err := discoveryClient.ServerGroups()
	if err != nil {
		return platform.Default, err
	}

	for _, group := range apiGroups.Groups {
		if group.Name == "route.openshift.io" {
			return platform.OpenShift, nil
		}
	}

	// Check for EKS
	for _, group := range apiGroups.Groups {
		if group.Name == "eks.amazonaws.com" {
			return platform.EKS, nil
		}
	}

	// Check for AKS
	namespace, err := clientset.CoreV1().Namespaces().Get(context.TODO(), "kube-system", metav1.GetOptions{})
	if err == nil {
		if _, exists := namespace.Labels["kubernetes.azure.com/managed"]; exists {
			return platform.AKS, nil
		}
	}

	// Check for GKE
	if err == nil {
		if _, exists := namespace.Labels["container.googleapis.com/cluster-name"]; exists {
			return platform.GKE, nil
		}
	}

	// Default to Kubernetes
	return platform.Kubernetes, nil
}
