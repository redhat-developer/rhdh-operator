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
	// Configuration for Backstage. Optional.
	Application *Application `json:"backstage,omitempty"`

	// Raw Runtime Objects configuration. For Advanced scenarios.
	RawRuntimeConfig RuntimeConfig `json:"rawRuntimeConfig,omitempty"`

	// Control the creation of a local PostgreSQL DB. Set to true if using for example an external Database for Backstage.
	// To use an external Database, you can provide your own app-config file (see the AppConfig field in the Application structure)
	// containing references to the Database connection information,
	// which might be supplied as environment variables (see the Env field) or extra-configuration files
	// (see the ExtraConfig field in the Application structure).
	// +optional
	//+kubebuilder:default=false
	SkipLocalDb *bool `json:"skipLocalDb,omitempty"`
}

type Application struct {

	// Optional Reference to a Secret to use for Backend Auth. A new one will be generated if not set.
	// This Secret is used to set an environment variable named 'APP_CONFIG_backend_auth_keys' in the
	// main container, which takes precedence over any 'backend.auth.keys' field defined
	// in default or custom application configuration files.
	// This is required for service-to-service auth and is shared by all backend plugins.
	//+optional
	BackendAuthSecretKeyRef *BackendAuthSecretKeyRef `json:"backendAuthSecretKeyRef,omitempty"`

	// References to existing app-configs Config objects, that will be mounted as files in the specified mount path.
	// Each element can be a reference to any ConfigMap or Secret,
	// and will be mounted inside the main application container under a dedicated directory containing the ConfigMap
	// or Secret name (relative to the specified mount path).
	// Additionally, each file will be passed as a `--config /path/to/secret_or_configmap/key` to the
	// main container args in the order of the entries defined in the AppConfigs list.
	// But bear in mind that for a single AppConfig element containing several files,
	// the order in which those files will be appended to the container args, the main container args cannot be guaranteed.
	// So if you want to pass multiple app-config files, it is recommended to pass one ConfigMap/Secret per app-config file.
	//+optional
	AppConfig *AppConfig `json:"appConfig,omitempty"`

	// Reference to an existing ConfigMap for Dynamic Plugins.
	// A new one will be generated with the default config if not set.
	// The ConfigMap object must have an existing key named: 'dynamic-plugins.yaml'.
	//+optional
	DynamicPluginsConfigMapRef string `json:"dynamicPluginsConfigMapRef,omitempty"`

	// References to existing Config objects to use as extra config files.
	// They will be mounted as files in the specified mount path.
	// Each element can be a reference to any ConfigMap or Secret.
	//+optional
	ExtraConfig *ExtraConfig `json:"extraConfig,omitempty"`

	// Environment variables to inject into the application containers.
	// Bear in mind not to put sensitive data here. Use EnvFrom instead.
	//+optional
	Env []Env `json:"env,omitempty"`

	// Environment variables to inject into the application containers, as references to existing ConfigMap or Secret objects.
	//+optional
	EnvFrom []EnvFrom `json:"envFrom,omitempty"`

	// Number of desired replicas to set in the Backstage Deployment.
	// Defaults to 1.
	//+optional
	//+kubebuilder:default=1
	Replicas *int32 `json:"replicas,omitempty"`

	// Image to use in all containers (including Init Containers)
	//+optional
	Image *string `json:"image,omitempty"`

	// Image Pull Secret to use in all containers (including Init Containers)
	//+optional
	ImagePullSecret *string `json:"imagePullSecret,omitempty"`
}

type BackendAuthSecretKeyRef struct {
	// Name of the secret to use for the backend auth
	//+kubebuilder:validation:Required
	Name string `json:"name"`

	// Key in the secret to use for the backend auth. Default value is: backend-secret
	// +optional
	//+kubebuilder:default=backend-secret
	Key string `json:"key,omitempty"`
}

type AppConfig struct {
	// Mount path for all app-config files listed in the ConfigMapRefs field
	// +optional
	// +kubebuilder:default=/opt/app-root/src
	MountPath string `json:"mountPath,omitempty"`

	// Names of ConfigMaps storing the app-config files. Will be mounted as files under the MountPath specified.
	// Bear in mind not to put sensitive data in those ConfigMaps. Instead, your app-config content can reference
	// environment variables (which you can set with the Env or EnvFrom fields) and/or include extra files (see the ExtraConfig field).
	// More details on https://backstage.io/docs/conf/writing/.
	// +optional
	ConfigMapRefs []string `json:"configMapRefs,omitempty"`
}

type ExtraConfig struct {
	// Mount path for all extra configuration files listed in the Items field
	// +optional
	// +kubebuilder:default=/opt/app-root/src
	MountPath string `json:"mountPath,omitempty"`

	// List of references to extra config Config objects.
	// +optional
	Items []ExtraConfigItem `json:"items,omitempty"`
}

type ExtraConfigItem struct {
	// ConfigMap containing one or more extra config files
	// +optional
	ConfigMapRef *ObjectRef `json:"configMapRef,omitempty"`

	// Secret containing one or more extra config files
	// +optional
	SecretRef *ObjectRef `json:"secretRef,omitempty"`
}

type DynamicPluginsConfig struct {
	// ConfigMap containing the dynamic plugins' configuration. It needs to have a key named: "dynamic-plugins.yaml".
	// ConfigMapRef will be used if both ConfigMapRef and SecretRef are provided.
	// +optional
	ConfigMapRef *ObjectRef `json:"configMapRef,omitempty"`

	// Secret containing the dynamic plugins' configuration. It needs to have a key named: "dynamic-plugins.yaml".
	// ConfigMapRef will be used if both ConfigMapRef and SecretRef are provided.
	// +optional
	SecretRef *ObjectRef `json:"secretRef,omitempty"`
}

type ObjectRef struct {
	// Name of the object referenced.
	//+kubebuilder:validation:Required
	Name string `json:"name"`
}

type Env struct {
	// Name of the environment variable
	//+kubebuilder:validation:Required
	Name string `json:"name"`

	// Value of the environment variable
	//+kubebuilder:validation:Required
	Value string `json:"value"`
}

type EnvFrom struct {
	// ConfigMap containing the environment variables to inject
	// +optional
	ConfigMapRef *ObjectRef `json:"configMapRef,omitempty"`

	// Secret containing the environment variables to inject
	// +optional
	SecretRef *ObjectRef `json:"secretRef,omitempty"`
}

type Postgresql struct {
	// Control the creation of a local PostgreSQL DB. Set to false if using for example an external Database for Backstage.
	// To use an external Database, you can provide your own app-config file (see the AppConfig field) containing references
	// to the Database connection information, which might be supplied as environment variables (see the Env field) or
	// extra-configuration files (see the ExtraConfig field in the Application structure).
	// +optional
	//+kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`
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
