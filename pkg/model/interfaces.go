package model

import (
	"github.com/redhat-developer/rhdh-operator/api"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MergeConfigFunc defines how config files from multiple sources (base + flavours) should be merged
// Returns the merged objects or an error
type MergeConfigFunc func(sources []configSource, scheme runtime.Scheme, platformExt string) ([]client.Object, error)

// Registered Object configuring Backstage runtime model
type ObjectConfig struct {
	// Factory to create the object
	ObjectFactory ObjectFactory
	// Unique key identifying the "kind" of Object which also is the name of config file.
	// For example: "deployment.yaml" containing configuration of Backstage Deployment
	Key string
	// Single or multiple object
	Multiple bool
	// MergeFunc defines how configs from multiple flavours are merged
	// nil means no flavour merging (base config only)
	MergeFunc MergeConfigFunc
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
