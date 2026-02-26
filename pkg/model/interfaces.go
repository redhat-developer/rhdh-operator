package model

import (
	"github.com/redhat-developer/rhdh-operator/api"

	"k8s.io/apimachinery/pkg/runtime"
)

// FlavourMergePolicy defines how config files from multiple flavours should be merged
type FlavourMergePolicy int

const (
	// FlavourMergePolicyNoFlavour indicates this config should only come from base default-config
	// Flavours are not expected to provide their own version of this file
	// Used for core infrastructure configs like deployment.yaml, service.yaml, db-*.yaml
	FlavourMergePolicyNoFlavour FlavourMergePolicy = iota

	// FlavourMergePolicyArrayMerge merges arrays by a key field (e.g., package name)
	// Used for dynamic-plugins.yaml where plugins are merged by package name
	FlavourMergePolicyArrayMerge

	// FlavourMergePolicyMultiObject creates separate objects for each flavour
	// Used for data files like app-config.yaml, configmap-envs.yaml
	// Each flavour gets its own ConfigMap/Secret with a flavour-prefixed name
	FlavourMergePolicyMultiObject
)

// Registered Object configuring Backstage runtime model
type ObjectConfig struct {
	// Factory to create the object
	ObjectFactory ObjectFactory
	// Unique key identifying the "kind" of Object which also is the name of config file.
	// For example: "deployment.yaml" containing configuration of Backstage Deployment
	Key string
	// Single or multiple object
	Multiple bool
	// FlavourMergePolicy defines how configs from multiple flavours are merged
	FlavourMergePolicy FlavourMergePolicy
}

// Interface for Runtime Objects factory method
type ObjectFactory interface {
	newBackstageObject() RuntimeObject
}

// Abstraction for the model Backstage object taking part in deployment
type RuntimeObject interface {
	// Object underlying Kubernetes object
	Object() runtime.Object
	// setObject sets object
	setObject(object runtime.Object)
	// adds runtime object to the model
	// returns false if the object was not added to the model (not configured)
	addToModel(model *BackstageModel, backstage api.Backstage) (bool, error)
	// at this stage all the information is added to the model
	// this step is for updating the final references and validate the object
	updateAndValidate(backstage api.Backstage) error
	// sets object name, labels and other necessary meta information
	setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme)
}

type ExternalConfigContributor interface {
	// addExternalConfig adds external configuration to deployment
	addExternalConfig(spec api.BackstageSpec) error
}
