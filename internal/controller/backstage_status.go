package controller

import (
	"context"
	"fmt"

	bs "github.com/redhat-developer/rhdh-operator/api/v1alpha4"
	"github.com/redhat-developer/rhdh-operator/pkg/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//func (r *BackstageReconciler) setDeploymentStatus(ctx context.Context, backstage *bs.Backstage) {
//	deploy := &appsv1.Deployment{}
//	if err := r.Get(ctx, types.NamespacedName{Name: model.DeploymentName(backstage.Name), Namespace: backstage.GetNamespace()}, deploy); err != nil {
//		setStatusCondition(backstage, bs.BackstageConditionTypeDeployed, metav1.ConditionFalse, bs.BackstageConditionReasonFailed, err.Error())
//		return
//	}
//
//	if deploy.Status.ReadyReplicas == deploy.Status.Replicas {
//		setStatusCondition(backstage, bs.BackstageConditionTypeDeployed, metav1.ConditionTrue, bs.BackstageConditionReasonDeployed, "")
//	} else {
//		msg := "Deployment status:"
//		for _, c := range deploy.Status.Conditions {
//			if c.Type == appsv1.DeploymentAvailable {
//				msg += " Available: " + c.Message
//			} else if c.Type == appsv1.DeploymentProgressing {
//				msg += " Progressing: " + c.Message
//			}
//		}
//		setStatusCondition(backstage, bs.BackstageConditionTypeDeployed, metav1.ConditionFalse, bs.BackstageConditionReasonInProgress, msg)
//	}
//}

//func (r *BackstageReconciler) setDeploymentStatus(ctx context.Context, backstage *bs.Backstage, backstageModel model.BackstageModel) {
//
//	if backstageModel.GetDeploymentGVK() == appsv1.SchemeGroupVersion.WithKind("Deployment") {
//		deploy := &appsv1.Deployment{}
//		if err := r.Get(ctx, types.NamespacedName{Name: model.DeploymentName(backstage.Name), Namespace: backstage.GetNamespace()}, deploy); err != nil {
//			setStatusCondition(backstage, bs.BackstageConditionTypeDeployed, metav1.ConditionFalse, bs.BackstageConditionReasonFailed, err.Error())
//		} else {
//			state, msg := deploymentState(deploy)
//			setStatusCondition(backstage, bs.BackstageConditionTypeDeployed, metav1.ConditionFalse, state, msg)
//		}
//	} else {
//		deploy := &appsv1.StatefulSet{}
//		if err := r.Get(ctx, types.NamespacedName{Name: model.DeploymentName(backstage.Name), Namespace: backstage.GetNamespace()}, deploy); err != nil {
//			setStatusCondition(backstage, bs.BackstageConditionTypeDeployed, metav1.ConditionFalse, bs.BackstageConditionReasonFailed, err.Error())
//		} else {
//			state, msg := statefulSetState(deploy)
//			setStatusCondition(backstage, bs.BackstageConditionTypeDeployed, metav1.ConditionFalse, state, msg)
//		}
//	}
//
//}

func (r *BackstageReconciler) setDeploymentStatus(ctx context.Context, backstage *bs.Backstage, backstageModel model.BackstageModel) {
	var obj client.Object
	var resolveState func(client.Object) (bs.BackstageConditionReason, string)

	switch backstageModel.GetDeploymentGVK() {
	case appsv1.SchemeGroupVersion.WithKind("StatefulSet"):
		obj = &appsv1.StatefulSet{}
		resolveState = func(o client.Object) (bs.BackstageConditionReason, string) {
			return statefulSetState(o.(*appsv1.StatefulSet))
		}
	default:
		obj = &appsv1.Deployment{}
		resolveState = func(o client.Object) (bs.BackstageConditionReason, string) {
			return deploymentState(o.(*appsv1.Deployment))
		}
	}

	if err := r.Get(ctx, types.NamespacedName{Name: model.DeploymentName(backstage.Name), Namespace: backstage.GetNamespace()}, obj); err != nil {
		setStatusCondition(backstage, bs.BackstageConditionTypeDeployed, metav1.ConditionFalse, bs.BackstageConditionReasonFailed, err.Error())
		return
	}

	state, msg := resolveState(obj)
	setStatusCondition(backstage, bs.BackstageConditionTypeDeployed, metav1.ConditionFalse, state, msg)
}

func setStatusCondition(backstage *bs.Backstage, condType bs.BackstageConditionType, status metav1.ConditionStatus, reason bs.BackstageConditionReason, msg string) {
	meta.SetStatusCondition(&backstage.Status.Conditions, metav1.Condition{
		Type:               string(condType),
		Status:             status,
		LastTransitionTime: metav1.Time{},
		Reason:             string(reason),
		Message:            msg,
	})
}

func deploymentState(deploy *appsv1.Deployment) (state bs.BackstageConditionReason, msg string) {
	if deploy.Status.ReadyReplicas == *deploy.Spec.Replicas {
		return bs.BackstageConditionReasonDeployed, ""
	}

	if len(deploy.Status.Conditions) == 0 {
		return bs.BackstageConditionReasonInProgress, "no conditions reported yet"
	}

	// Prefer explicit failure indicators
	for _, c := range deploy.Status.Conditions {
		if c.Type == appsv1.DeploymentReplicaFailure && c.Status == corev1.ConditionTrue {
			return bs.BackstageConditionReasonFailed, c.Message
		}
	}

	// Fallback: aggregate condition info as in-progress
	msg = ""
	for _, c := range deploy.Status.Conditions {
		msg += fmt.Sprintf(" %s=%s(%s);", c.Type, c.Status, c.Message)
	}
	return bs.BackstageConditionReasonInProgress, msg
}

func statefulSetState(deploy *appsv1.StatefulSet) (state bs.BackstageConditionReason, msg string) {
	desired := int32(1)
	if deploy.Spec.Replicas != nil {
		desired = *deploy.Spec.Replicas
	}

	if deploy.Status.ReadyReplicas == desired {
		return bs.BackstageConditionReasonDeployed, ""
	}

	if len(deploy.Status.Conditions) == 0 {
		return bs.BackstageConditionReasonInProgress, "no conditions reported yet"
	}

	// Prefer explicit failure indicators
	for _, c := range deploy.Status.Conditions {
		if string(c.Type) == "ReplicaFailure" && c.Status == corev1.ConditionTrue {
			return bs.BackstageConditionReasonFailed, c.Message
		}
	}

	// Fallback: aggregate condition info as in-progress
	msg = ""
	for _, c := range deploy.Status.Conditions {
		msg += fmt.Sprintf(" %s=%s(%s);", c.Type, c.Status, c.Message)
	}
	return bs.BackstageConditionReasonInProgress, msg
}
