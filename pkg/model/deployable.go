package model

import (
	"errors"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const unsupportedType = "unsupported type for Deployable object: "

// Deployable is an interface for Kubernetes objects that can deploy Backstage Pods (Deployment, StatefulSet)
type Deployable interface {
	// GetObject returns the underlying Kubernetes object
	GetObject() client.Object
	// PodSpec returns the spec.template.spec of the deployable object
	PodSpec() *corev1.PodSpec
	// PodObjectMeta returns the spec.template.metadata of the deployable object
	PodObjectMeta() *metav1.ObjectMeta
	// SpecSelector returns the spec.selector of the deployable object
	SpecSelector() *metav1.LabelSelector
	// SpecReplicas returns the spec.replicas of the deployable object
	SpecReplicas() *int32
	// ConvertTo converts the deployable object to the specified kind (Deployment or StatefulSet)
	ConvertTo(kind string) (Deployable, error)
	// SetEmpty sets the deployable object to an empty object of its type
	SetEmpty()
}

//const unsupportedType = "unsupported deployable type: "

// CreateDeployable creates a new Deployable object
func CreateDeployable(obj runtime.Object) (Deployable, error) {
	switch o := obj.(type) {
	case *appv1.StatefulSet:
		return &StatefulSetObj{Obj: o}, nil
	case *appv1.Deployment:
		return &DeploymentObj{Obj: o}, nil
	default:
		return nil, errors.New(unsupportedType + obj.GetObjectKind().GroupVersionKind().Kind)
	}
}
