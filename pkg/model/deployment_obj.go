package model

import (
	"errors"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// compile-time check
var _ Deployable = (*DeploymentObj)(nil)

type DeploymentObj struct {
	Obj *appv1.Deployment
}

func (d *DeploymentObj) ConvertTo(kind string) (Deployable, error) {
	switch kind {
	case "StatefulSet":
		return CreateDeployable(toStatefulSet(d.Obj))
	case "Deployment":
		return d, nil
	}
	return nil, errors.New(unsupportedType + kind)
}

func (d *DeploymentObj) GetObject() client.Object {
	return d.Obj
}

func (d *DeploymentObj) SetEmpty() {
	d.Obj = &appv1.Deployment{}
}

func (d *DeploymentObj) PodSpec() *corev1.PodSpec {
	return &d.Obj.Spec.Template.Spec
}

func (d *DeploymentObj) PodObjectMeta() *metav1.ObjectMeta {
	return &d.Obj.Spec.Template.ObjectMeta
}

func (d *DeploymentObj) SpecSelector() *metav1.LabelSelector {
	if d.Obj.Spec.Selector == nil {
		d.Obj.Spec.Selector = &metav1.LabelSelector{}
	}
	if d.Obj.Spec.Selector.MatchLabels == nil {
		d.Obj.Spec.Selector.MatchLabels = map[string]string{}
	}
	return d.Obj.Spec.Selector
}

func (d *DeploymentObj) SpecReplicas() *int32 {
	return d.Obj.Spec.Replicas
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
