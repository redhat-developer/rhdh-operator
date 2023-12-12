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
	Application *Application `json:"application,omitempty"`

	// Raw Runtime Objects configuration. For Advanced scenarios.
	RawRuntimeConfig RuntimeConfig `json:"rawRuntimeConfig,omitempty"`

	// Control the creation of a local PostgreSQL DB. Set to false if using for example an external Database for Backstage.
	// To use an external Database, you can provide your own app-config file (see the AppConfig field in the Application structure)
	// containing references to the Database connection information,
	// which might be supplied as environment variables (see the ExtraEnvs field) or extra-configuration files
	// (see the ExtraFiles field in the Application structure).
	// +optional
	//+kubebuilder:default=true
	EnableLocalDb *bool `json:"enableLocalDb,omitempty"`
}

type Application struct {
	// References to existing app-configs ConfigMap objects, that will be mounted as files in the specified mount path.
	// Each element can be a reference to any ConfigMap or Secret,
	// and will be mounted inside the main application container under a specified mount directory.
	// Additionally, each file will be passed as a `--config /mount/path/to/configmap/key` to the
	// main container args in the order of the entries defined in the AppConfigs list.
	// But bear in mind that for a single ConfigMap element containing several filenames,
	// the order in which those files will be appended to the main container args cannot be guaranteed.
	// So if you want to pass multiple app-config files, it is recommended to pass one ConfigMap per app-config file.
	// +optional
	AppConfig *AppConfig `json:"appConfig,omitempty"`

	// Reference to an existing ConfigMap for Dynamic Plugins.
	// A new one will be generated with the default config if not set.
	// The ConfigMap object must have an existing key named: 'dynamic-plugins.yaml'.
	// +optional
	DynamicPluginsConfigMapName string `json:"dynamicPluginsConfigMapName,omitempty"`

	// References to existing Config objects to use as extra config files.
	// They will be mounted as files in the specified mount path.
	// Each element can be a reference to any ConfigMap or Secret.
	// +optional
	ExtraFiles *ExtraFiles `json:"extraFiles,omitempty"`

	// Extra environment variables
	// +optional
	ExtraEnvs *ExtraEnvs `json:"extraEnvs,omitempty"`

	// Number of desired replicas to set in the Backstage Deployment.
	// Defaults to 1.
	// +optional
	//+kubebuilder:default=1
	Replicas *int32 `json:"replicas,omitempty"`

	// Image to use in all containers (including Init Containers)
	// +optional
	Image *string `json:"image,omitempty"`

	// Image Pull Secrets to use in all containers (including Init Containers)
	// +optional
	ImagePullSecrets []string `json:"imagePullSecrets,omitempty"`
}

type AppConfig struct {
	// Mount path for all app-config files listed in the ConfigMapRefs field
	// +optional
	// +kubebuilder:default=/opt/app-root/src
	MountPath string `json:"mountPath,omitempty"`

	// List of ConfigMaps storing the app-config files. Will be mounted as files under the MountPath specified.
	// For each item in this array, if a key is not specified, it means that all keys in the ConfigMap will be mounted as files.
	// Otherwise, only the specified key will be mounted as a file.
	// Bear in mind not to put sensitive data in those ConfigMaps. Instead, your app-config content can reference
	// environment variables (which you can set with the ExtraEnvs field) and/or include extra files (see the ExtraFiles field).
	// More details on https://backstage.io/docs/conf/writing/.
	// +optional
	ConfigMaps []ObjectKeyRef `json:"configMaps,omitempty"`
}

type ExtraFiles struct {
	// Mount path for all extra configuration files listed in the Items field
	// +optional
	// +kubebuilder:default=/opt/app-root/src
	MountPath string `json:"mountPath,omitempty"`

	// List of references to ConfigMaps objects mounted as extra files under the MountPath specified.
	// For each item in this array, if a key is not specified, it means that all keys in the ConfigMap will be mounted as files.
	// Otherwise, only the specified key will be mounted as a file.
	// +optional
	ConfigMaps []ObjectKeyRef `json:"configMaps,omitempty"`

	// List of references to Secrets objects mounted as extra files under the MountPath specified.
	// For each item in this array, if a key is not specified, it means that all keys in the Secret will be mounted as files.
	// Otherwise, only the specified key will be mounted as a file.
	// +optional
	Secrets []ObjectKeyRef `json:"secrets,omitempty"`
}

type ExtraEnvs struct {
	// List of references to ConfigMaps objects to inject as additional environment variables.
	// For each item in this array, if a key is not specified, it means that all keys in the ConfigMap will be injected as additional environment variables.
	// Otherwise, only the specified key will be injected as an additional environment variable.
	// +optional
	ConfigMaps []ObjectKeyRef `json:"configMaps,omitempty"`

	// List of references to Secrets objects to inject as additional environment variables.
	// For each item in this array, if a key is not specified, it means that all keys in the Secret will be injected as additional environment variables.
	// Otherwise, only the specified key will be injected as environment variable.
	// +optional
	Secrets []ObjectKeyRef `json:"secrets,omitempty"`

	// List of name and value pairs to add as environment variables.
	// +optional
	Envs []Env `json:"envs,omitempty"`
}

type ObjectKeyRef struct {
	// Name of the object
	// We support only ConfigMaps and Secrets.
	//+kubebuilder:validation:Required
	Name string `json:"name"`

	// Key in the object
	// +optional
	Key string `json:"key,omitempty"`
}

type Env struct {
	// Name of the environment variable
	//+kubebuilder:validation:Required
	Name string `json:"name"`

	// Value of the environment variable
	//+kubebuilder:validation:Required
	Value string `json:"value"`
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
