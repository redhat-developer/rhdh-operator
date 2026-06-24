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
	// Object returns the underlying Kubernetes object.
	// Only objects that should be applied are added to the model, so this never returns nil.
	Object() runtime.Object
	// GetKey returns the unique key identifying this object type (e.g., "deployment.yaml")
	GetKey() string
	// addToModel initializes the object from config and conditionally registers it in model.
	// config parameter is the chosen configuration (overlay OR default, selected by runtime.go)
	// Objects are only added to model (via setRuntimeObject) if they should be applied to cluster.
	addToModel(model *BackstageModel, backstage api.Backstage, config runtime.Object, scheme *runtime.Scheme) error
	// updateAndValidate wires dependencies, creates dynamic objects, validates, and updates metadata.
	// This is called after all objects are added to model, so cross-references are safe.
	updateAndValidate(backstage api.Backstage, scheme *runtime.Scheme) error
}
