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
	//LocalDb     LocalDbConfig `json:"localDb,omitempty"`

	//+kubebuilder:validation:XEmbeddedResource
	//Deployment appsv1.Deployment `json:"deployment,omitempty"`

	//+kubebuilder:validation:XEmbeddedResource
	//Service corev1.Service `json:"service,omitempty"`
}

type RuntimeConfig struct {
	BackstageConfigName string `json:"backstageConfig,omitempty"`
	LocalDbConfigName   string `json:"localDbConfig,omitempty"`
}

//// Configuration works like this (for the time):
//// * if some object PV, PVC, etc defined - it is taken as a basis
//// * otherwise default will be taken (TODO: move defaults to Operator's ConfigMap?)
//// * and it is also possible to ovewrite some with Parameters if any (TODO: do we need it?)
//// TODO do we need to move this to ConfigMap to not to overload CR?
//type LocalDbConfig struct {
//	Parameters LocalDbParameters `json:"parameters,omitempty"`
//	//+kubebuilder:validation:XEmbeddedResource
//	PersistentVolume corev1.PersistentVolume `json:"persistentVolume,omitempty"`
//	//+kubebuilder:validation:XEmbeddedResource
//	PersistentVolumeClaim corev1.PersistentVolumeClaim `json:"persistentVolumeClaim,omitempty"`
//	//+kubebuilder:validation:XEmbeddedResource
//	Deployment appsv1.Deployment `json:"deployment,omitempty"`
//	//+kubebuilder:validation:XEmbeddedResource
//	Service corev1.Service `json:"service,omitempty"`
//}

//type LocalDbParameters struct {
//	DeploymentName  string `json:"deploymentName,omitempty"`
//	Replicas        int    `json:"replicas,omitempty"`
//	StorageCapacity string `json:"capacity,omitempty"`
//	SecretRefName   string `json:"secretRefName,omitempty"`
//	Image           string `json:"image,omitempty"`
//	PullPolicy      string `json:"pullPolicy,omitempty"`
//}

// BackstageStatus defines the observed state of Backstage
type BackstageStatus struct {
	//TODO
	BackstageState string `json:"backstageState,omitempty"`
	//LocalDb        LocalDbStatus `json:"localDb,omitempty"`
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

// Backstage is the Schema for the backstagedeployments API
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
