package v1alpha5

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// Example: registry.redhat.io/rhdh/plugin-catalog:1.5
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

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=dhpc
// +kubebuilder:printcolumn:name="Ref",type=string,JSONPath=`.spec.source.ref`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DevHubPluginCatalog is the Schema for the devhubplugincatalogs API.
// It defines a source for plugin catalogs that can be fetched from OCI registries.
type DevHubPluginCatalog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec DevHubPluginCatalogSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// DevHubPluginCatalogList contains a list of DevHubPluginCatalog
type DevHubPluginCatalogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DevHubPluginCatalog `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DevHubPluginCatalog{}, &DevHubPluginCatalogList{})
}
