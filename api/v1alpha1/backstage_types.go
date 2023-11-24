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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	RuntimeConditionRunning string = "RuntimeRunning"
	RuntimeConditionSynced  string = "RuntimeSyncedWithConfig"
)

// BackstageSpec defines the desired state of Backstage
type BackstageSpec struct {
	// Backstage application AppConfigs
	AppConfigs []Config `json:"appConfigs,omitempty"`

	// Optional Backend Auth Secret Name. A new one will be generated if not set.
	// This Secret is used to set an environment variable named 'APP_CONFIG_backend_auth_keys' in the
	// main container, which takes precedence over any 'backend.auth.keys' field defined
	// in default or custom application configuration files.
	// This is required for service-to-service auth and is shared by all backend plugins.
	BackendAuthSecretRef BackendAuthSecretRef `json:"backendAuthSecretRef,omitempty"`

	// Dynamic Plugins configuration
	DynamicPluginsConfig Config `json:"dynamicPluginsConfig,omitempty"`

	// Raw Runtime Objects configuration
	RawRuntimeConfig RuntimeConfig `json:"rawRuntimeConfig,omitempty"`

	//+kubebuilder:default=false
	SkipLocalDb bool `json:"skipLocalDb,omitempty"`
}

type Config struct {
	Name string `json:"name,omitempty"`
	//+kubebuilder:validation:Enum=ConfigMap;Secret
	Kind string `json:"kind,omitempty"`
}

type BackendAuthSecretRef struct {
	// Name of the secret to use for the backend auth
	Name string `json:"name,omitempty"`

	// Key in the secret to use for the backend auth. Default value is: backend-secret
	//+kubebuilder:default=backend-secret
	Key string `json:"key,omitempty"`
}

type RuntimeConfig struct {
	// Name of ConfigMap containing Backstage runtime objects configuration
	BackstageConfigName string `json:"backstageConfig,omitempty"`
	// Name of ConfigMap containing LocalDb (P|ostgreSQL) runtime objects configuration
	LocalDbConfigName string `json:"localDbConfig,omitempty"`
}

// BackstageStatus defines the observed state of Backstage
type BackstageStatus struct {
	// Conditions is the list of conditions describing the state of the runtime
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Backstage is the Schema for the backstages API
type Backstage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackstageSpec   `json:"spec,omitempty"`
	Status BackstageStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BackstageList contains a list of Backstage
type BackstageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Backstage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Backstage{}, &BackstageList{})
}
