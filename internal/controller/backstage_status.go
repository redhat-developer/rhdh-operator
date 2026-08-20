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
// Returns true if deployment is ready, false otherwise.
func (r *BackstageReconciler) reconcileStatus(ctx context.Context, backstage *api.Backstage, backstageModel model.BackstageModel) bool {
	// Set Runtime condition first (Pod/Container level) - more immediate state detection
	// This catches crash loops before deployment status has chance to show brief "ready" windows
	runtimeHealthy := r.setRuntimeCondition(ctx, backstage)

	// Set Deployed condition (Deployment/StatefulSet level)
	deployedReady := r.setDeployedCondition(ctx, backstage, backstageModel)

	// Both deployment and runtime must be healthy for overall readiness
	isReady := deployedReady && runtimeHealthy

	// Set enabled plugins only when fully healthy
	if isReady {
		setEnabledPlugins(backstage, backstageModel)
	} else {
		backstage.Status.Plugins = nil
	}

	return isReady
}

// setDeployedCondition sets the Deployed condition based on Deployment/StatefulSet status.
func (r *BackstageReconciler) setDeployedCondition(ctx context.Context, backstage *api.Backstage, backstageModel model.BackstageModel) bool {
	var obj client.Object
	var resolveState func(client.Object) (metav1.ConditionStatus, api.BackstageConditionReason, string)

	switch backstageModel.GetDeploymentGVK() {
	case appsv1.SchemeGroupVersion.WithKind("StatefulSet"):
		obj = &appsv1.StatefulSet{}
		resolveState = func(o client.Object) (metav1.ConditionStatus, api.BackstageConditionReason, string) {
			return statefulSetState(o.(*appsv1.StatefulSet))
		}
	default:
		obj = &appsv1.Deployment{}
		resolveState = func(o client.Object) (metav1.ConditionStatus, api.BackstageConditionReason, string) {
			return deploymentState(o.(*appsv1.Deployment))
		}
	}

	if err := r.Get(ctx, types.NamespacedName{Name: model.DeploymentName(backstage.Name), Namespace: backstage.GetNamespace()}, obj); err != nil {
		setStatusCondition(backstage, api.BackstageConditionTypeDeployed, metav1.ConditionFalse, api.BackstageConditionReasonFailed, err.Error())
		return false
	}

	status, reason, msg := resolveState(obj)
	if isIdled(backstage) {
		msg += " (Idled)"
	}
	setStatusCondition(backstage, api.BackstageConditionTypeDeployed, status, reason, msg)

	return status == metav1.ConditionTrue
}

// setRuntimeCondition sets the Runtime condition based on Pod/Container status.
// Provides detailed container-level info for debugging (complements Deployed condition).
// Returns true if runtime is healthy (all containers running), false otherwise.
func (r *BackstageReconciler) setRuntimeCondition(ctx context.Context, backstage *api.Backstage) bool {
	var status metav1.ConditionStatus
	var reason api.BackstageConditionReason
	var msg string

	podList := &corev1.PodList{}
	labelSelector := client.MatchingLabels{
		model.BackstageAppLabel: utils.BackstageAppLabelValue(backstage.Name),
	}

	if err := r.List(ctx, podList, client.InNamespace(backstage.Namespace), labelSelector); err != nil {
		status, reason, msg = metav1.ConditionFalse, api.BackstageConditionReasonPending, "unable to list pods"
	} else if len(podList.Items) == 0 {
		status, reason, msg = metav1.ConditionFalse, api.BackstageConditionReasonPending, "no pods found"
	} else {
		status, reason, msg = checkPodStates(podList.Items)
	}

	// Idled instances have 0 pods by design - consider healthy to avoid requeue loop
	isHealthy := status == metav1.ConditionTrue || isIdled(backstage)
	if isHealthy {
		status = metav1.ConditionTrue
	}
	setStatusCondition(backstage, api.BackstageConditionTypeRuntime, status, reason, msg)

	return isHealthy
}

// checkPodStates checks pods for errors, returns Running/True if all healthy.
func checkPodStates(pods []corev1.Pod) (metav1.ConditionStatus, api.BackstageConditionReason, string) {
	// Sort pods by creation time, newest first
	sort.Slice(pods, func(i, j int) bool {
		return pods[j].CreationTimestamp.Before(&pods[i].CreationTimestamp)
	})

	for _, pod := range pods {
		// Check pod phase for terminal failures
		if pod.Status.Phase == corev1.PodFailed {
			return metav1.ConditionFalse, api.BackstageConditionReasonContainerFailed, fmt.Sprintf("pod %q failed", pod.Name)
		}

		// Check init containers
		for _, cs := range pod.Status.InitContainerStatuses {
			if reason, msg := checkContainerState(cs, true); reason != "" {
				return metav1.ConditionFalse, reason, msg
			}
		}

		// Check main containers
		for _, cs := range pod.Status.ContainerStatuses {
			if reason, msg := checkContainerState(cs, false); reason != "" {
				return metav1.ConditionFalse, reason, msg
			}
		}

		// If pod is not Running and no container issues found, it's still pending
		// This catches the case where pod exists but no container statuses yet
		if pod.Status.Phase != corev1.PodRunning {
			return metav1.ConditionFalse, api.BackstageConditionReasonPending, fmt.Sprintf("pod %q is %s", pod.Name, pod.Status.Phase)
		}
	}

	return metav1.ConditionTrue, api.BackstageConditionReasonRunning, ""
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

	// Check for crash loop: container has restarted with a previous failure
	// For main containers: only report if not currently Ready (container may have recovered)
	// For init containers: always report (they should complete successfully, not restart)
	if cs.RestartCount > 0 && cs.LastTerminationState.Terminated != nil {
		lastTerm := cs.LastTerminationState.Terminated
		if lastTerm.ExitCode != 0 && (isInit || !cs.Ready) {
			msg := fmt.Sprintf("%s %q crashed (restart #%d), last exit code %d", prefix, cs.Name, cs.RestartCount, lastTerm.ExitCode)
			if lastTerm.Message != "" {
				msg += ": " + lastTerm.Message
			} else if lastTerm.Reason != "" {
				msg += " (" + lastTerm.Reason + ")"
			}
			return api.BackstageConditionReasonContainerFailed, msg
		}
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

func removeStatusCondition(backstage *api.Backstage, condType api.BackstageConditionType) {
	meta.RemoveStatusCondition(&backstage.Status.Conditions, string(condType))
}

func isIdled(backstage *api.Backstage) bool {
	return backstage.GetAnnotations()[model.IdleAnnotation] == "true"
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

func deploymentState(deploy *appsv1.Deployment) (metav1.ConditionStatus, api.BackstageConditionReason, string) {
	desired := int32(1)
	if deploy.Spec.Replicas != nil {
		desired = *deploy.Spec.Replicas
	}
	if deploy.Status.ReadyReplicas == desired {
		return metav1.ConditionTrue, api.BackstageConditionReasonDeployed, fmt.Sprintf("%d/%d replicas ready", desired, desired)
	}

	if len(deploy.Status.Conditions) == 0 {
		return metav1.ConditionFalse, api.BackstageConditionReasonInProgress, "no conditions reported yet"
	}

	// Prefer explicit failure indicators
	for _, c := range deploy.Status.Conditions {
		if c.Type == appsv1.DeploymentReplicaFailure && c.Status == corev1.ConditionTrue {
			return metav1.ConditionFalse, api.BackstageConditionReasonFailed, c.Message
		}
	}

	return metav1.ConditionFalse, api.BackstageConditionReasonInProgress, fmt.Sprintf("%d/%d replicas ready", deploy.Status.ReadyReplicas, desired)
}

func statefulSetState(sts *appsv1.StatefulSet) (metav1.ConditionStatus, api.BackstageConditionReason, string) {
	desired := int32(1)
	if sts.Spec.Replicas != nil {
		desired = *sts.Spec.Replicas
	}

	if sts.Status.ReadyReplicas == desired && sts.Status.CurrentReplicas == sts.Status.UpdatedReplicas {
		return metav1.ConditionTrue, api.BackstageConditionReasonDeployed, fmt.Sprintf("%d/%d replicas ready", desired, desired)
	}

	if len(sts.Status.Conditions) == 0 {
		return metav1.ConditionFalse, api.BackstageConditionReasonInProgress, "no conditions reported yet"
	}

	return metav1.ConditionFalse, api.BackstageConditionReasonInProgress, fmt.Sprintf("%d/%d replicas ready", sts.Status.ReadyReplicas, desired)
}
