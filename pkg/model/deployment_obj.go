package model

import (
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const unsupportedType = "unsupported type for Deployment object: "

type DeploymentObj struct {
	Obj client.Object
}

// setKind converts the underlying object to the specified kind ("StatefulSet" or "Deployment")
func (d *DeploymentObj) setKind(kind string) {
	switch d.Obj.(type) {
	case *appv1.StatefulSet:
		if kind == "StatefulSet" {
			return
		}
		d.Obj = toDeployment(d.Obj.(*appv1.StatefulSet))
	case *appv1.Deployment:
		if kind == "Deployment" {
			return
		}
		d.Obj = toStatefulSet(d.Obj.(*appv1.Deployment))
	default:
		panic(unsupportedType + d.Obj.GetObjectKind().GroupVersionKind().Kind)
	}
}

// setObject sets the underlying object to the provided runtime.Object
func (d *DeploymentObj) setObject(obj runtime.Object) {
	switch o := obj.(type) {
	case *appv1.StatefulSet:
		d.Obj = o
	case *appv1.Deployment:
		d.Obj = o
	default:
		panic(unsupportedType + obj.GetObjectKind().GroupVersionKind().Kind)
	}
}

// setEmpty sets the underlying object to an empty object of the same type
func (d *DeploymentObj) setEmpty() {
	switch d.Obj.(type) {
	case *appv1.StatefulSet:
		d.Obj = &appv1.StatefulSet{}
	case *appv1.Deployment:
		d.Obj = &appv1.Deployment{}
	default:
		panic(unsupportedType + d.Obj.GetObjectKind().GroupVersionKind().Kind)
	}
}

// PodSpec returns the PodSpec of the underlying object
func (d *DeploymentObj) PodSpec() *corev1.PodSpec {

	switch obj := d.Obj.(type) {
	case *appv1.StatefulSet:
		return &obj.Spec.Template.Spec
	case *appv1.Deployment:
		return &obj.Spec.Template.Spec
	default:
		panic(unsupportedType + obj.GetObjectKind().GroupVersionKind().Kind)
	}
}

// podObjectMeta returns the ObjectMeta of the Pod template of the underlying object
func (d *DeploymentObj) podObjectMeta() *metav1.ObjectMeta {

	switch obj := d.Obj.(type) {
	case *appv1.StatefulSet:
		return &obj.Spec.Template.ObjectMeta
	case *appv1.Deployment:
		return &obj.Spec.Template.ObjectMeta
	default:
		panic(unsupportedType + obj.GetObjectKind().GroupVersionKind().Kind)
	}
}

// specSelector returns the LabelSelector of the underlying object
func (d *DeploymentObj) specSelector() *metav1.LabelSelector {
	switch obj := d.Obj.(type) {
	case *appv1.StatefulSet:
		if obj.Spec.Selector == nil {
			obj.Spec.Selector = &metav1.LabelSelector{}
		}
		if obj.Spec.Selector.MatchLabels == nil {
			obj.Spec.Selector.MatchLabels = map[string]string{}
		}
		return obj.Spec.Selector
	case *appv1.Deployment:
		if obj.Spec.Selector == nil {
			obj.Spec.Selector = &metav1.LabelSelector{}
		}
		if obj.Spec.Selector.MatchLabels == nil {
			obj.Spec.Selector.MatchLabels = map[string]string{}
		}
		return obj.Spec.Selector
	default:
		panic(unsupportedType + obj.GetObjectKind().GroupVersionKind().Kind)
	}

}

// SpecReplicas returns the Replicas field of the underlying object
func (d *DeploymentObj) SpecReplicas() *int32 {

	switch obj := d.Obj.(type) {
	case *appv1.StatefulSet:
		return obj.Spec.Replicas
	case *appv1.Deployment:
		return obj.Spec.Replicas
	default:
		panic("unsupported type for DeploymentObj " + obj.GetObjectKind().GroupVersionKind().Kind)
	}
}

// StatusConditions returns the Status.Conditions field of the underlying object
func (d *DeploymentObj) StatusConditions() []appv1.DeploymentCondition {
	switch obj := d.Obj.(type) {
	case *appv1.Deployment:
		return obj.Status.Conditions
	default:
		panic("unsupported type for DeploymentObj " + obj.GetObjectKind().GroupVersionKind().Kind)
	}
}

// toStatefulSet converts a Deployment to a StatefulSet
func toStatefulSet(dep *appv1.Deployment) *appv1.StatefulSet {
	ss := &appv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: *dep.ObjectMeta.DeepCopy(),
		Spec: appv1.StatefulSetSpec{
			Replicas:    dep.Spec.Replicas,
			Selector:    dep.Spec.Selector,
			Template:    dep.Spec.Template,
			ServiceName: "",
		},
	}

	if ss.Spec.Selector == nil {
		ss.Spec.Selector = &metav1.LabelSelector{}
	}
	if ss.Spec.Selector.MatchLabels == nil {
		ss.Spec.Selector.MatchLabels = map[string]string{}
	}
	return ss
}

// toDeployment converts a StatefulSet to a Deployment
func toDeployment(ss *appv1.StatefulSet) *appv1.Deployment {
	dep := &appv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: *ss.ObjectMeta.DeepCopy(),
		Spec: appv1.DeploymentSpec{
			Replicas: ss.Spec.Replicas,
			Selector: ss.Spec.Selector,
			Template: ss.Spec.Template,
		},
	}

	if dep.Spec.Selector == nil {
		dep.Spec.Selector = &metav1.LabelSelector{}
	}
	if dep.Spec.Selector.MatchLabels == nil {
		dep.Spec.Selector.MatchLabels = map[string]string{}
	}
	return dep
}
