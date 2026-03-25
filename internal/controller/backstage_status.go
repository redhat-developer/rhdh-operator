package controller

import (
	"context"
	"fmt"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *BackstageReconciler) setDeploymentStatus(ctx context.Context, backstage *api.Backstage, backstageModel model.BackstageModel) {
	var obj client.Object
	var resolveState func(client.Object) (api.BackstageConditionReason, string)

	switch backstageModel.GetDeploymentGVK() {
	case appsv1.SchemeGroupVersion.WithKind("StatefulSet"):
		obj = &appsv1.StatefulSet{}
		resolveState = func(o client.Object) (api.BackstageConditionReason, string) {
			return statefulSetState(o.(*appsv1.StatefulSet))
		}
	default:
		obj = &appsv1.Deployment{}
		resolveState = func(o client.Object) (api.BackstageConditionReason, string) {
			return deploymentState(o.(*appsv1.Deployment))
		}
	}

	if err := r.Get(ctx, types.NamespacedName{Name: model.DeploymentName(backstage.Name), Namespace: backstage.GetNamespace()}, obj); err != nil {
		setStatusCondition(backstage, api.BackstageConditionTypeDeployed, metav1.ConditionFalse, api.BackstageConditionReasonFailed, err.Error())
		return
	}

	state, msg := resolveState(obj)
	status := metav1.ConditionFalse
	if state == api.BackstageConditionReasonDeployed {
		status = metav1.ConditionTrue
	}
	setStatusCondition(backstage, bs.BackstageConditionTypeDeployed, status, state, msg)
}

func setStatusCondition(backstage *api.Backstage, condType api.BackstageConditionType, status metav1.ConditionStatus, reason api.BackstageConditionReason, msg string) {
	meta.SetStatusCondition(&backstage.Status.Conditions, metav1.Condition{
		Type:               string(condType),
		Status:             status,
		LastTransitionTime: metav1.Time{},
		Reason:             string(reason),
		Message:            msg,
	})
}

func deploymentState(deploy *appsv1.Deployment) (state api.BackstageConditionReason, msg string) {
	desired := int32(1)
	if deploy.Spec.Replicas != nil {
		desired = *deploy.Spec.Replicas
	}
	if deploy.Status.ReadyReplicas == desired {
		return api.BackstageConditionReasonDeployed, ""
	}

	if len(deploy.Status.Conditions) == 0 {
		return api.BackstageConditionReasonInProgress, "no conditions reported yet"
	}

	// Prefer explicit failure indicators
	for _, c := range deploy.Status.Conditions {
		if c.Type == appsv1.DeploymentReplicaFailure && c.Status == corev1.ConditionTrue {
			return api.BackstageConditionReasonFailed, c.Message
		}
	}

	// Fallback: aggregate condition info as in-progress
	msg = ""
	for _, c := range deploy.Status.Conditions {
		msg += fmt.Sprintf(" %s=%s(%s);", c.Type, c.Status, c.Message)
	}
	return api.BackstageConditionReasonInProgress, msg
}

func statefulSetState(deploy *appsv1.StatefulSet) (state api.BackstageConditionReason, msg string) {
	desired := int32(1)
	if deploy.Spec.Replicas != nil {
		desired = *deploy.Spec.Replicas
	}

	//if deploy.Status.ReadyReplicas == desired {
	if deploy.Status.ReadyReplicas == desired && deploy.Status.CurrentReplicas == deploy.Status.UpdatedReplicas {
		return api.BackstageConditionReasonDeployed, ""
	}

	if len(deploy.Status.Conditions) == 0 {
		return api.BackstageConditionReasonInProgress, "no conditions reported yet"
	}

	// Fallback: aggregate condition info as in-progress
	msg = ""
	for _, c := range deploy.Status.Conditions {
		msg += fmt.Sprintf(" %s=%s(%s);", c.Type, c.Status, c.Message)
	}
	return api.BackstageConditionReasonInProgress, msg
}
