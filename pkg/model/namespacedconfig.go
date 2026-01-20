package model

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NamespacedConfig holds user-provided resources discovered via labels in the namespace
// These are shared defaults (not instance-specific) that complement operator's built-in defaults
type NamespacedConfig struct {
	// Secrets to be injected as environment variables
	// Discovered via rhdh.redhat.com/default-config label (empty value or flavor name)
	EnvSecrets []*corev1.Secret
}

func NewNamespacedConfig() NamespacedConfig {
	return NamespacedConfig{
		EnvSecrets: []*corev1.Secret{},
	}
}

// ListOptions returns client.ListOptions configured to select secrets with default-config label
func (nc *NamespacedConfig) ListOptions(namespace string) *client.ListOptions {
	selector := labels.NewSelector()
	requirement, _ := labels.NewRequirement(DefaultConfigLabel, selection.Exists, nil)
	selector = selector.Add(*requirement)

	return &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: selector,
	}
}
