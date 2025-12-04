package model

import (
	"errors"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// compile-time check
var _ Deployable = (*StatefulSetObj)(nil)

type StatefulSetObj struct {
	Obj *appv1.StatefulSet
}

func (d *StatefulSetObj) ConvertTo(kind string) (Deployable, error) {
	if kind == "Deployment" {
		return CreateDeployable(toDeployment(d.Obj))
	} else if kind == "StatefulSet" {
		return d, nil
	}
	return nil, errors.New(unsupportedType + kind)
}

func (d *StatefulSetObj) GetObject() client.Object {
	return d.Obj
}

func (d *StatefulSetObj) SetEmpty() {
	d.Obj = &appv1.StatefulSet{}
}

func (d *StatefulSetObj) PodSpec() *corev1.PodSpec {
	return &d.Obj.Spec.Template.Spec
}

func (d *StatefulSetObj) PodObjectMeta() *metav1.ObjectMeta {
	return &d.Obj.Spec.Template.ObjectMeta
}

func (d *StatefulSetObj) SpecSelector() *metav1.LabelSelector {
	if d.Obj.Spec.Selector == nil {
		d.Obj.Spec.Selector = &metav1.LabelSelector{}
	}
	if d.Obj.Spec.Selector.MatchLabels == nil {
		d.Obj.Spec.Selector.MatchLabels = map[string]string{}
	}
	return d.Obj.Spec.Selector
}

func (d *StatefulSetObj) SpecReplicas() *int32 {
	return d.Obj.Spec.Replicas
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
