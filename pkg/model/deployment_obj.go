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

func (d *DeploymentObj) setKind(kind string) {
	switch d.Obj.(type) {
	case *appv1.StatefulSet:
		d.Obj.(*appv1.StatefulSet).Kind = kind
	case *appv1.Deployment:
		d.Obj.(*appv1.Deployment).Kind = kind
	default:
		panic(unsupportedType + d.Obj.GetObjectKind().GroupVersionKind().Kind)
	}
}

func (d *DeploymentObj) setObject(obj runtime.Object) {
	switch obj.(type) {
	case *appv1.StatefulSet:
		d.Obj = obj.(*appv1.StatefulSet)
	case *appv1.Deployment:
		d.Obj = obj.(*appv1.Deployment)
	default:
		panic(unsupportedType + obj.GetObjectKind().GroupVersionKind().Kind)
	}
}

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

func (d *DeploymentObj) StatusConditions() []appv1.DeploymentCondition {
	switch obj := d.Obj.(type) {
	case *appv1.Deployment:
		return obj.Status.Conditions
	default:
		panic("unsupported type for DeploymentObj " + obj.GetObjectKind().GroupVersionKind().Kind)
	}
}
