package controller

import (
	"context"
	"fmt"
	"sort"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// reconcileStatus updates the Backstage CR status based on deployment and runtime state.
// Returns true if all conditions are satisfied, false otherwise.
func (r *BackstageReconciler) reconcileStatus(ctx context.Context, backstage *api.Backstage, backstageModel model.BackstageModel) bool {
	// Set Deployed condition (Deployment/StatefulSet level)
	deployedReady := r.setDeployedCondition(ctx, backstage, backstageModel)

	// Set Runtime condition (Pod/Container level)
	runtimeHealthy := r.setRuntimeCondition(ctx, backstage)

	// Set enabled plugins in status
	setEnabledPlugins(backstage, backstageModel)

	// Both conditions must be satisfied. Currently they track similar state,
	// but Runtime could be extended to check additional health indicators
	// (e.g., restart counts, probe failures) that Deployed doesn't capture.
	return deployedReady && runtimeHealthy
}

// setDeployedCondition sets the Deployed condition based on Deployment/StatefulSet status.
func (r *BackstageReconciler) setDeployedCondition(ctx context.Context, backstage *api.Backstage, backstageModel model.BackstageModel) bool {
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
		return false
	}

	state, msg := resolveState(obj)
	isReady := state == api.BackstageConditionReasonDeployed
	status := metav1.ConditionFalse
	if isReady {
		status = metav1.ConditionTrue
	}
	setStatusCondition(backstage, api.BackstageConditionTypeDeployed, status, state, msg)

	return isReady
}

// setRuntimeCondition sets the Runtime condition based on Pod/Container status.
func (r *BackstageReconciler) setRuntimeCondition(ctx context.Context, backstage *api.Backstage) bool {
	reason, msg := r.getRuntimeState(ctx, backstage)

	isHealthy := reason == api.BackstageConditionReasonRunning
	status := metav1.ConditionFalse
	if isHealthy {
		status = metav1.ConditionTrue
	}
	setStatusCondition(backstage, api.BackstageConditionTypeRuntime, status, reason, msg)

	return isHealthy
}

// getRuntimeState checks pod/container state and returns appropriate reason and message.
func (r *BackstageReconciler) getRuntimeState(ctx context.Context, backstage *api.Backstage) (api.BackstageConditionReason, string) {
	podList := &corev1.PodList{}
	labelSelector := client.MatchingLabels{
		model.BackstageAppLabel: utils.BackstageAppLabelValue(backstage.Name),
	}
	if err := r.List(ctx, podList, client.InNamespace(backstage.Namespace), labelSelector); err != nil {
		return api.BackstageConditionReasonPending, "unable to list pods"
	}

	if len(podList.Items) == 0 {
		return api.BackstageConditionReasonPending, "no pods found"
	}

	// Sort pods by creation time, newest first
	pods := podList.Items
	sort.Slice(pods, func(i, j int) bool {
		return pods[j].CreationTimestamp.Before(&pods[i].CreationTimestamp)
	})

	// Check pods - single pass for errors and readiness
	for _, pod := range pods {
		// Check init containers
		for _, cs := range pod.Status.InitContainerStatuses {
			if reason, msg := checkContainerState(cs, true); reason != "" {
				return reason, msg
			}
		}

		// Check main containers
		for _, cs := range pod.Status.ContainerStatuses {
			if reason, msg := checkContainerState(cs, false); reason != "" {
				return reason, msg
			}
		}
	}

	return api.BackstageConditionReasonRunning, ""
}

// checkContainerState checks a single container status and returns reason/message if not healthy.
func checkContainerState(cs corev1.ContainerStatus, isInit bool) (api.BackstageConditionReason, string) {
	prefix := "container"
	if isInit {
		prefix = "init container"
	}

	// Check for failures (Waiting with error reason)
	if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
		if cs.State.Waiting.Reason == "PodInitializing" {
			return "", "" // Normal state, not an error
		}
		return api.BackstageConditionReasonContainerFailed, fmt.Sprintf("%s %q: %s", prefix, cs.Name, cs.State.Waiting.Reason)
	}

	// Check for terminated with error
	if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
		msg := fmt.Sprintf("%s %q failed with exit code %d", prefix, cs.Name, cs.State.Terminated.ExitCode)
		if cs.State.Terminated.Message != "" {
			msg += ": " + cs.State.Terminated.Message
		} else if cs.State.Terminated.Reason != "" {
			msg += " (" + cs.State.Terminated.Reason + ")"
		}
		return api.BackstageConditionReasonContainerFailed, msg
	}

	// For init containers: must be terminated successfully
	if isInit {
		if cs.State.Running != nil {
			return api.BackstageConditionReasonPending, fmt.Sprintf("%s %q running", prefix, cs.Name)
		}
		if cs.State.Terminated == nil {
			return api.BackstageConditionReasonPending, fmt.Sprintf("%s %q not completed", prefix, cs.Name)
		}
	} else {
		// For main containers: must be ready
		if !cs.Ready {
			return api.BackstageConditionReasonPending, fmt.Sprintf("%s %q not ready", prefix, cs.Name)
		}
	}

	return "", "" // Healthy
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

// setEnabledPlugins sets the list of enabled plugin names in status.
func setEnabledPlugins(backstage *api.Backstage, backstageModel model.BackstageModel) {
	enabledPlugins := backstageModel.GetEnabledPlugins()
	pluginNames := make([]string, 0, len(enabledPlugins))
	for _, p := range enabledPlugins {
		pluginNames = append(pluginNames, p.Package)
	}
	sort.Strings(pluginNames)
	backstage.Status.Plugins = pluginNames
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

	return api.BackstageConditionReasonInProgress, fmt.Sprintf("%d/%d replicas ready", deploy.Status.ReadyReplicas, desired)
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

	return api.BackstageConditionReasonInProgress, fmt.Sprintf("%d/%d replicas ready", deploy.Status.ReadyReplicas, desired)
}
