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
	// References to existing app-configs Config objects.
	// Each element can be a reference to any ConfigMap or Secret,
	// and will be mounted inside the main application container under a dedicated directory containing the ConfigMap
	// or Secret name. Additionally, each file will be passed as a `--config /path/to/secret_or_configmap/key` to the
	// main container args in the order of the entries defined in the AppConfigs list.
	// But bear in mind that for a single AppConfig element containing several files,
	// the order in which those files will be appended to the container args, the main container args cannot be guaranteed.
	// So if you want to pass multiple app-config files, it is recommended to pass one ConfigMap/Secret per app-config file.
	AppConfigs []AppConfigRef `json:"appConfigs,omitempty"`

	// Optional Backend Auth Secret Name. A new one will be generated if not set.
	// This Secret is used to set an environment variable named 'APP_CONFIG_backend_auth_keys' in the
	// main container, which takes precedence over any 'backend.auth.keys' field defined
	// in default or custom application configuration files.
	// This is required for service-to-service auth and is shared by all backend plugins.
	BackendAuthSecretRef BackendAuthSecretRef `json:"backendAuthSecretRef,omitempty"`

	// Reference to an existing configuration object for Dynamic Plugins.
	// This can be a reference to any ConfigMap or Secret,
	// but the object must have an existing key named: 'dynamic-plugins.yaml'
	DynamicPluginsConfig DynamicPluginsConfigRef `json:"dynamicPluginsConfig,omitempty"`

	// Raw Runtime Objects configuration
	RawRuntimeConfig RuntimeConfig `json:"rawRuntimeConfig,omitempty"`

	//+kubebuilder:default=false
	SkipLocalDb bool `json:"skipLocalDb,omitempty"`
}

type AppConfigRef struct {
	// Name of an existing App Config object
	Name string `json:"name,omitempty"`

	// Type of the existing App Config object, either ConfigMap or Secret
	//+kubebuilder:validation:Enum=ConfigMap;Secret
	Kind string `json:"kind,omitempty"`
}

type DynamicPluginsConfigRef struct {
	// Name of the Dynamic Plugins config object
	Name string `json:"name,omitempty"`

	// Type of the Dynamic Plugins config object, either ConfigMap or Secret
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
