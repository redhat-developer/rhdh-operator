package controller

import (
	"testing"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestCheckContainerState_MainContainer(t *testing.T) {
	tests := []struct {
		name           string
		status         corev1.ContainerStatus
		expectedReason api.BackstageConditionReason
		expectedMsg    string
	}{
		{
			name: "ready container",
			status: corev1.ContainerStatus{
				Name:  "backstage",
				Ready: true,
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
				},
			},
			expectedReason: "",
			expectedMsg:    "",
		},
		{
			name: "not ready container",
			status: corev1.ContainerStatus{
				Name:  "backstage",
				Ready: false,
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
				},
			},
			expectedReason: api.BackstageConditionReasonPending,
			expectedMsg:    `container "backstage" not ready`,
		},
		{
			name: "waiting with ImagePullBackOff",
			status: corev1.ContainerStatus{
				Name: "backstage",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason: "ImagePullBackOff",
					},
				},
			},
			expectedReason: api.BackstageConditionReasonContainerFailed,
			expectedMsg:    `container "backstage": ImagePullBackOff`,
		},
		{
			name: "waiting with CrashLoopBackOff",
			status: corev1.ContainerStatus{
				Name: "backstage",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason: "CrashLoopBackOff",
					},
				},
			},
			expectedReason: api.BackstageConditionReasonContainerFailed,
			expectedMsg:    `container "backstage": CrashLoopBackOff`,
		},
		{
			name: "terminated with error",
			status: corev1.ContainerStatus{
				Name: "backstage",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 1,
						Reason:   "Error",
					},
				},
			},
			expectedReason: api.BackstageConditionReasonContainerFailed,
			expectedMsg:    `container "backstage" failed with exit code 1 (Error)`,
		},
		{
			name: "terminated with message",
			status: corev1.ContainerStatus{
				Name: "backstage",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 137,
						Message:  "OOMKilled",
					},
				},
			},
			expectedReason: api.BackstageConditionReasonContainerFailed,
			expectedMsg:    `container "backstage" failed with exit code 137: OOMKilled`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, msg := checkContainerState(tt.status, false)
			assert.Equal(t, tt.expectedReason, reason)
			assert.Equal(t, tt.expectedMsg, msg)
		})
	}
}

func TestCheckContainerState_InitContainer(t *testing.T) {
	tests := []struct {
		name           string
		status         corev1.ContainerStatus
		expectedReason api.BackstageConditionReason
		expectedMsg    string
	}{
		{
			name: "terminated successfully",
			status: corev1.ContainerStatus{
				Name: "install-plugins",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 0,
					},
				},
			},
			expectedReason: "",
			expectedMsg:    "",
		},
		{
			name: "still running",
			status: corev1.ContainerStatus{
				Name: "install-plugins",
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
				},
			},
			expectedReason: api.BackstageConditionReasonPending,
			expectedMsg:    `init container "install-plugins" running`,
		},
		{
			name: "waiting PodInitializing",
			status: corev1.ContainerStatus{
				Name: "install-plugins",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason: "PodInitializing",
					},
				},
			},
			expectedReason: "",
			expectedMsg:    "",
		},
		{
			name: "waiting with error",
			status: corev1.ContainerStatus{
				Name: "install-plugins",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason: "ImagePullBackOff",
					},
				},
			},
			expectedReason: api.BackstageConditionReasonContainerFailed,
			expectedMsg:    `init container "install-plugins": ImagePullBackOff`,
		},
		{
			name: "terminated with error",
			status: corev1.ContainerStatus{
				Name: "install-plugins",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 1,
						Message:  "plugin download failed",
					},
				},
			},
			expectedReason: api.BackstageConditionReasonContainerFailed,
			expectedMsg:    `init container "install-plugins" failed with exit code 1: plugin download failed`,
		},
		{
			name: "no state yet",
			status: corev1.ContainerStatus{
				Name:  "install-plugins",
				State: corev1.ContainerState{},
			},
			expectedReason: api.BackstageConditionReasonPending,
			expectedMsg:    `init container "install-plugins" not completed`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, msg := checkContainerState(tt.status, true)
			assert.Equal(t, tt.expectedReason, reason)
			assert.Equal(t, tt.expectedMsg, msg)
		})
	}
}

func TestDeploymentState(t *testing.T) {
	tests := []struct {
		name           string
		deployment     *appsv1.Deployment
		expectedReason api.BackstageConditionReason
		expectedMsg    string
	}{
		{
			name: "all replicas ready",
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas: 1,
				},
			},
			expectedReason: api.BackstageConditionReasonDeployed,
			expectedMsg:    "",
		},
		{
			name: "multiple replicas ready",
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas: 3,
				},
			},
			expectedReason: api.BackstageConditionReasonDeployed,
			expectedMsg:    "",
		},
		{
			name: "partial replicas ready",
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas: 1,
					Conditions: []appsv1.DeploymentCondition{
						{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue},
					},
				},
			},
			expectedReason: api.BackstageConditionReasonInProgress,
			expectedMsg:    "1/3 replicas ready",
		},
		{
			name: "no replicas ready yet",
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas: 0,
					Conditions: []appsv1.DeploymentCondition{
						{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue},
					},
				},
			},
			expectedReason: api.BackstageConditionReasonInProgress,
			expectedMsg:    "0/1 replicas ready",
		},
		{
			name: "no conditions yet",
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas: 0,
					Conditions:    []appsv1.DeploymentCondition{},
				},
			},
			expectedReason: api.BackstageConditionReasonInProgress,
			expectedMsg:    "no conditions reported yet",
		},
		{
			name: "replica failure",
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas: 0,
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:    appsv1.DeploymentReplicaFailure,
							Status:  corev1.ConditionTrue,
							Message: "quota exceeded",
						},
					},
				},
			},
			expectedReason: api.BackstageConditionReasonFailed,
			expectedMsg:    "quota exceeded",
		},
		{
			name: "nil replicas defaults to 1",
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: nil,
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas: 1,
				},
			},
			expectedReason: api.BackstageConditionReasonDeployed,
			expectedMsg:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, msg := deploymentState(tt.deployment)
			assert.Equal(t, tt.expectedReason, reason)
			assert.Equal(t, tt.expectedMsg, msg)
		})
	}
}

func TestStatefulSetState(t *testing.T) {
	tests := []struct {
		name           string
		statefulset    *appsv1.StatefulSet
		expectedReason api.BackstageConditionReason
		expectedMsg    string
	}{
		{
			name: "all replicas ready and updated",
			statefulset: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.StatefulSetStatus{
					ReadyReplicas:   1,
					CurrentReplicas: 1,
					UpdatedReplicas: 1,
				},
			},
			expectedReason: api.BackstageConditionReasonDeployed,
			expectedMsg:    "",
		},
		{
			name: "ready but not updated",
			statefulset: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.StatefulSetStatus{
					ReadyReplicas:   1,
					CurrentReplicas: 1,
					UpdatedReplicas: 0,
					Conditions: []appsv1.StatefulSetCondition{
						{Type: "Ready"},
					},
				},
			},
			expectedReason: api.BackstageConditionReasonInProgress,
			expectedMsg:    "1/1 replicas ready",
		},
		{
			name: "partial replicas ready",
			statefulset: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.StatefulSetStatus{
					ReadyReplicas:   2,
					CurrentReplicas: 3,
					UpdatedReplicas: 3,
					Conditions: []appsv1.StatefulSetCondition{
						{Type: "Ready"},
					},
				},
			},
			expectedReason: api.BackstageConditionReasonInProgress,
			expectedMsg:    "2/3 replicas ready",
		},
		{
			name: "no conditions yet",
			statefulset: &appsv1.StatefulSet{
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.StatefulSetStatus{
					ReadyReplicas: 0,
					Conditions:    []appsv1.StatefulSetCondition{},
				},
			},
			expectedReason: api.BackstageConditionReasonInProgress,
			expectedMsg:    "no conditions reported yet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, msg := statefulSetState(tt.statefulset)
			assert.Equal(t, tt.expectedReason, reason)
			assert.Equal(t, tt.expectedMsg, msg)
		})
	}
}

func TestSetStatusCondition(t *testing.T) {
	backstage := &api.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bs",
			Namespace: "test-ns",
		},
	}

	// Set initial condition
	setStatusCondition(backstage, api.BackstageConditionTypeDeployed, metav1.ConditionFalse, api.BackstageConditionReasonInProgress, "starting")

	assert.Len(t, backstage.Status.Conditions, 1)
	assert.Equal(t, "Deployed", backstage.Status.Conditions[0].Type)
	assert.Equal(t, metav1.ConditionFalse, backstage.Status.Conditions[0].Status)
	assert.Equal(t, "DeployInProgress", backstage.Status.Conditions[0].Reason)
	assert.Equal(t, "starting", backstage.Status.Conditions[0].Message)

	// Update existing condition
	setStatusCondition(backstage, api.BackstageConditionTypeDeployed, metav1.ConditionTrue, api.BackstageConditionReasonDeployed, "")

	assert.Len(t, backstage.Status.Conditions, 1)
	assert.Equal(t, metav1.ConditionTrue, backstage.Status.Conditions[0].Status)
	assert.Equal(t, "Deployed", backstage.Status.Conditions[0].Reason)

	// Add another condition type
	setStatusCondition(backstage, api.BackstageConditionTypeRuntime, metav1.ConditionTrue, api.BackstageConditionReasonRunning, "")

	assert.Len(t, backstage.Status.Conditions, 2)
}
