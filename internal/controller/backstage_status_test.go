package controller

import (
	"testing"
	"time"

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
		{
			name: "crash loop - running but has previous failure",
			status: corev1.ContainerStatus{
				Name:         "backstage",
				RestartCount: 3,
				Ready:        false,
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
				},
				LastTerminationState: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 1,
						Message:  "connection refused",
					},
				},
			},
			expectedReason: api.BackstageConditionReasonContainerFailed,
			expectedMsg:    `container "backstage" crashed (restart #3), last exit code 1: connection refused`,
		},
		{
			name: "recovered after crash - running and ready with restart history",
			status: corev1.ContainerStatus{
				Name:         "backstage",
				RestartCount: 2,
				Ready:        true,
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
				},
				LastTerminationState: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 1,
						Reason:   "Error",
					},
				},
			},
			expectedReason: "",
			expectedMsg:    "",
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
		{
			name: "crash loop - running but has previous failure",
			status: corev1.ContainerStatus{
				Name:         "install-plugins",
				RestartCount: 2,
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{},
				},
				LastTerminationState: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 1,
						Reason:   "Error",
					},
				},
			},
			expectedReason: api.BackstageConditionReasonContainerFailed,
			expectedMsg:    `init container "install-plugins" crashed (restart #2), last exit code 1 (Error)`,
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
		expectedStatus metav1.ConditionStatus
		expectedReason api.BackstageConditionReason
		expectedMsg    string
	}{
		{
			name: "all replicas ready and updated",
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas:   1,
					UpdatedReplicas: 1,
				},
			},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: api.BackstageConditionReasonDeployed,
			expectedMsg:    "1/1 replicas ready",
		},
		{
			name: "multiple replicas ready and updated",
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas:   3,
					UpdatedReplicas: 3,
				},
			},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: api.BackstageConditionReasonDeployed,
			expectedMsg:    "3/3 replicas ready",
		},
		{
			name: "partial replicas ready during rollout",
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas:   1,
					UpdatedReplicas: 1,
					Conditions: []appsv1.DeploymentCondition{
						{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue},
					},
				},
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: api.BackstageConditionReasonInProgress,
			expectedMsg:    "1/3 replicas ready, 1/3 updated",
		},
		{
			name: "no replicas ready yet",
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas:   0,
					UpdatedReplicas: 0,
					Conditions: []appsv1.DeploymentCondition{
						{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue},
					},
				},
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: api.BackstageConditionReasonInProgress,
			expectedMsg:    "0/1 replicas ready, 0/1 updated",
		},
		{
			name: "no conditions yet",
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(1)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas:   0,
					UpdatedReplicas: 1,
					Conditions:      []appsv1.DeploymentCondition{},
				},
			},
			expectedStatus: metav1.ConditionFalse,
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
					ReadyReplicas:   0,
					UpdatedReplicas: 1,
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:    appsv1.DeploymentReplicaFailure,
							Status:  corev1.ConditionTrue,
							Message: "quota exceeded",
						},
					},
				},
			},
			expectedStatus: metav1.ConditionFalse,
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
					ReadyReplicas:   1,
					UpdatedReplicas: 1,
				},
			},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: api.BackstageConditionReasonDeployed,
			expectedMsg:    "1/1 replicas ready",
		},
		{
			name: "rollout stalled - old pods ready, new pods failing",
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(2)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas:   2,
					UpdatedReplicas: 0,
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:   appsv1.DeploymentProgressing,
							Status: corev1.ConditionFalse,
							Reason: "ProgressDeadlineExceeded",
						},
					},
				},
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: api.BackstageConditionReasonRolloutStalled,
			expectedMsg:    "2/2 replicas ready, 0/2 updated (rollout stalled)",
		},
		{
			name: "rollout stalled - partial updated",
			deployment: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.DeploymentStatus{
					ReadyReplicas:   2,
					UpdatedReplicas: 1,
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:   appsv1.DeploymentProgressing,
							Status: corev1.ConditionFalse,
							Reason: "ProgressDeadlineExceeded",
						},
					},
				},
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: api.BackstageConditionReasonRolloutStalled,
			expectedMsg:    "2/3 replicas ready, 1/3 updated (rollout stalled)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, reason, msg := deploymentState(tt.deployment)
			assert.Equal(t, tt.expectedStatus, status)
			assert.Equal(t, tt.expectedReason, reason)
			assert.Equal(t, tt.expectedMsg, msg)
		})
	}
}

func TestStatefulSetState(t *testing.T) {
	tests := []struct {
		name           string
		statefulset    *appsv1.StatefulSet
		expectedStatus metav1.ConditionStatus
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
			expectedStatus: metav1.ConditionTrue,
			expectedReason: api.BackstageConditionReasonDeployed,
			expectedMsg:    "1/1 replicas ready",
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
			expectedStatus: metav1.ConditionFalse,
			expectedReason: api.BackstageConditionReasonInProgress,
			expectedMsg:    "1/1 replicas ready, 0/1 updated",
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
			expectedStatus: metav1.ConditionFalse,
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
			expectedStatus: metav1.ConditionFalse,
			expectedReason: api.BackstageConditionReasonInProgress,
			expectedMsg:    "no conditions reported yet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, reason, msg := statefulSetState(tt.statefulset)
			assert.Equal(t, tt.expectedStatus, status)
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

func TestCheckPodStates(t *testing.T) {
	tests := []struct {
		name           string
		pods           []corev1.Pod
		expectedStatus metav1.ConditionStatus
		expectedReason api.BackstageConditionReason
		expectedMsg    string
	}{
		{
			name: "single healthy pod",
			pods: []corev1.Pod{
				{
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						ContainerStatuses: []corev1.ContainerStatus{
							{Name: "backstage", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
						},
					},
				},
			},
			expectedStatus: metav1.ConditionTrue,
			expectedReason: api.BackstageConditionReasonRunning,
			expectedMsg:    "",
		},
		{
			name: "pod in failed phase",
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "backstage-abc"},
					Status:     corev1.PodStatus{Phase: corev1.PodFailed},
				},
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: api.BackstageConditionReasonContainerFailed,
			expectedMsg:    `pod "backstage-abc" failed`,
		},
		{
			name: "pod pending phase",
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "backstage-abc"},
					Status:     corev1.PodStatus{Phase: corev1.PodPending},
				},
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: api.BackstageConditionReasonPending,
			expectedMsg:    `pod "backstage-abc" is Pending`,
		},
		{
			name: "init container failed",
			pods: []corev1.Pod{
				{
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
						InitContainerStatuses: []corev1.ContainerStatus{
							{
								Name: "install-plugins",
								State: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{ExitCode: 1, Reason: "Error"},
								},
							},
						},
					},
				},
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: api.BackstageConditionReasonContainerFailed,
			expectedMsg:    `init container "install-plugins" failed with exit code 1 (Error)`,
		},
		{
			name: "main container crash loop",
			pods: []corev1.Pod{
				{
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						ContainerStatuses: []corev1.ContainerStatus{
							{
								Name:         "backstage",
								Ready:        false,
								RestartCount: 5,
								State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
								LastTerminationState: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{ExitCode: 137, Message: "OOMKilled"},
								},
							},
						},
					},
				},
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: api.BackstageConditionReasonContainerFailed,
			expectedMsg:    `container "backstage" crashed (restart #5), last exit code 137: OOMKilled`,
		},
		{
			name: "multiple pods - newest checked first",
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "backstage-old",
						CreationTimestamp: metav1.Time{Time: metav1.Now().Add(-time.Hour)},
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						ContainerStatuses: []corev1.ContainerStatus{
							{Name: "backstage", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "backstage-new",
						CreationTimestamp: metav1.Now(),
					},
					Status: corev1.PodStatus{Phase: corev1.PodFailed},
				},
			},
			expectedStatus: metav1.ConditionFalse,
			expectedReason: api.BackstageConditionReasonContainerFailed,
			expectedMsg:    `pod "backstage-new" failed`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, reason, msg := checkPodStates(tt.pods)
			assert.Equal(t, tt.expectedStatus, status)
			assert.Equal(t, tt.expectedReason, reason)
			assert.Equal(t, tt.expectedMsg, msg)
		})
	}
}

func TestIsIdled(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{
			name:        "no annotations",
			annotations: nil,
			expected:    false,
		},
		{
			name:        "empty annotations",
			annotations: map[string]string{},
			expected:    false,
		},
		{
			name:        "idle annotation true",
			annotations: map[string]string{"rhdh.redhat.com/idle": "true"},
			expected:    true,
		},
		{
			name:        "idle annotation false",
			annotations: map[string]string{"rhdh.redhat.com/idle": "false"},
			expected:    false,
		},
		{
			name:        "idle annotation other value",
			annotations: map[string]string{"rhdh.redhat.com/idle": "yes"},
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backstage := &api.Backstage{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotations,
				},
			}
			assert.Equal(t, tt.expected, isIdled(backstage))
		})
	}
}

func TestRemoveStatusCondition(t *testing.T) {
	backstage := &api.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bs",
			Namespace: "test-ns",
		},
	}

	// Set multiple conditions
	setStatusCondition(backstage, api.BackstageConditionTypeDeployed, metav1.ConditionTrue, api.BackstageConditionReasonDeployed, "ready")
	setStatusCondition(backstage, api.BackstageConditionTypeConfig, metav1.ConditionFalse, api.BackstageConditionReasonInvalid, "invalid config")
	setStatusCondition(backstage, api.BackstageConditionTypeRuntime, metav1.ConditionTrue, api.BackstageConditionReasonRunning, "")

	assert.Len(t, backstage.Status.Conditions, 3)

	// Remove Config condition
	removeStatusCondition(backstage, api.BackstageConditionTypeConfig)

	assert.Len(t, backstage.Status.Conditions, 2)

	// Verify Config condition is gone
	for _, c := range backstage.Status.Conditions {
		assert.NotEqual(t, string(api.BackstageConditionTypeConfig), c.Type)
	}

	// Remove non-existent condition should be no-op
	removeStatusCondition(backstage, api.BackstageConditionTypeConfig)
	assert.Len(t, backstage.Status.Conditions, 2)
}
