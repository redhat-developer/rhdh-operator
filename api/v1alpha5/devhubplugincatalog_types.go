package v1alpha5

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DevHubPluginCatalogSpec defines the desired state of DevHubPluginCatalog
type DevHubPluginCatalogSpec struct {
	// Source defines where to fetch the catalog from
	// +kubebuilder:validation:Required
	Source CatalogSource `json:"source"`
}

// CatalogSource defines the source of the plugin catalog
type CatalogSource struct {
	// Ref is the OCI artifact reference containing the catalog
	// Example: oci://registry.redhat.io/rhdh/plugin-catalog:1.5
	// +kubebuilder:validation:Required
	Ref string `json:"ref"`

	// PullSecret is a reference to a Secret containing registry credentials
	// +optional
	PullSecret *corev1.LocalObjectReference `json:"pullSecret,omitempty"`

	// CertificateAuthority is a reference to a ConfigMap key containing CA certificate
	// +optional
	CertificateAuthority *corev1.ConfigMapKeySelector `json:"certificateAuthority,omitempty"`

	// SkipTLSVerify skips TLS certificate verification (not recommended for production)
	// +optional
	// +kubebuilder:default=false
	SkipTLSVerify bool `json:"skipTLSVerify,omitempty"`
}

// DevHubPluginCatalogStatus defines the observed state of DevHubPluginCatalog
type DevHubPluginCatalogStatus struct {
	// Conditions represent the latest available observations of the catalog's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// Condition types for DevHubPluginCatalog
const (
	// ConditionTypeReady indicates whether the catalog has been successfully processed
	ConditionTypeReady = "Ready"
)

// Condition reasons for DevHubPluginCatalog
const (
	ConditionReasonProcessing = "Processing"
	ConditionReasonSucceeded  = "Succeeded"
	ConditionReasonFailed     = "Failed"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=dhpc
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ref",type=string,JSONPath=`.spec.source.ref`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DevHubPluginCatalog is the Schema for the devhubplugincatalogs API.
// It defines a source for plugin catalogs that can be fetched from OCI registries.
type DevHubPluginCatalog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DevHubPluginCatalogSpec   `json:"spec,omitempty"`
	Status DevHubPluginCatalogStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DevHubPluginCatalogList contains a list of DevHubPluginCatalog
type DevHubPluginCatalogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DevHubPluginCatalog `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &DevHubPluginCatalog{}, &DevHubPluginCatalogList{})
		return nil
	})
}
