package model

import (
	bsv1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha3"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	appsv1 "k8s.io/api/apps/v1"
)

type ObjectConfigOptions struct {
	Multiple bool
}

// Registered Object configuring Backstage runtime model
type ObjectConfig struct {
	// Factory to create the object
	ObjectFactory ObjectFactory
	// Unique key identifying the "kind" of Object which also is the name of config file.
	// For example: "deployment.yaml" containing configuration of Backstage Deployment
	Key string
	// Single or multiple object
	Multiple bool
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
	// EmptyObject an empty object: the same type as Object if Object is client.Object or Item type of Multiobject
	EmptyObject() client.Object
	// adds runtime object to the model
	// returns false if the object was not added to the model (not configured)
	addToModel(model *BackstageModel, backstage bsv1.Backstage) (bool, error)
	// at this stage all the information is updated
	// set the final references validates the object at the end of initialization
	validate(model *BackstageModel, backstage bsv1.Backstage) error
	// sets object name, labels and other necessary meta information
	setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme)
}

// BackstagePodContributor contributing to the pod as an Environment variables or mounting file/directory.
// Usually app-config related
type BackstagePodContributor interface {
	RuntimeObject
	updatePod(deployment *appsv1.Deployment)
}
