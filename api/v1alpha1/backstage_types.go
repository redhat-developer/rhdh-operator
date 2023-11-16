/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BackstageSpec defines the desired state of Backstage
type BackstageSpec struct {
	AppConfigs    []string      `json:"appConfigs,omitempty"`
	RuntimeConfig RuntimeConfig `json:"runtimeConfig,omitempty"`
	//+kubebuilder:default=false
	DryRun bool `json:"dryRun,omitempty"`

	//+kubebuilder:default=false
	SkipLocalDb bool `json:"skipLocalDb,omitempty"`
}

type RuntimeConfig struct {
	BackstageConfigName string `json:"backstageConfig,omitempty"`
	LocalDbConfigName   string `json:"localDbConfig,omitempty"`
}

// BackstageStatus defines the observed state of Backstage
type BackstageStatus struct {
	BackstageState string `json:"backstageState,omitempty"`
}

type LocalDbStatus struct {
	PersistentVolume LocalDbPersistentVolume `json:"PersistentVolume,omitempty"`
}

type LocalDbPersistentVolume struct {
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"`
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
