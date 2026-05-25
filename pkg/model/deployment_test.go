package model

import (
	"context"
	"testing"

	"github.com/redhat-developer/rhdh-operator/pkg/platform"

	appv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"k8s.io/utils/ptr"

	"github.com/redhat-developer/rhdh-operator/api"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

var deploymentTestBackstage = api.Backstage{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "bs",
		Namespace: "ns123",
	},
	Spec: api.BackstageSpec{
		Database: &api.Database{
			EnableLocalDb: ptr.To(false),
		},
		Application: &api.Application{},
	},
}

func TestWorkingDirMount(t *testing.T) {
	bs := *deploymentTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).
		addToDefaultConfig("deployment.yaml", "working-dir-mount.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)

	deployment := model.GetRuntimeObject(DeploymentKey).(*BackstageDeployment)
	assert.Equal(t, "/my/home", deployment.defaultMountPath())
	fileor := api.FileObjectRef{
		Name:      "test",
		MountPath: "subpath",
	}
	mp, sp := deployment.mountPath(fileor.MountPath, "", "")
	assert.Equal(t, "/my/home/subpath", mp)
	assert.False(t, sp)

}

// It tests the overriding image feature
func TestOverrideBackstageImage(t *testing.T) {

	bs := *deploymentTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).
		addToDefaultConfig("deployment.yaml", "sidecar-deployment.yaml")

	t.Setenv(BackstageImageEnvVar, "dummy")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)

	deployment := model.GetRuntimeObject(DeploymentKey).(*BackstageDeployment)
	assert.Equal(t, 2, len(deployment.podSpec().Containers))
	assert.Equal(t, "dummy", deployment.container().Image)
	assert.Equal(t, "dummy", deployment.podSpec().InitContainers[0].Image)
	assert.Equal(t, "busybox", deployment.podSpec().Containers[1].Image)

}

func TestSpecImagePullSecrets(t *testing.T) {
	bs := *deploymentTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).
		addToDefaultConfig("deployment.yaml", "ips-deployment.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)

	deployment := model.GetRuntimeObject(DeploymentKey).(*BackstageDeployment)
	// if imagepullsecrets not defined - default used
	assert.Equal(t, 2, len(deployment.podSpec().ImagePullSecrets))
	assert.Equal(t, "ips1", deployment.podSpec().ImagePullSecrets[0].Name)

	//bs.Spec.Application.ImagePullSecrets = []string{}
	//
	//testObj = createBackstageTest(bs).withDefaultConfig(true).
	//	addToDefaultConfig("deployment.yaml", "ips-deployment.yaml")
	//
	//model, err = InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	//assert.NoError(t, err)
	//
	//// if explicitly set empty slice - they are empty
	//assert.Equal(t, 0, len(model.backstageDeployment.deployment.Spec.Template.Spec.ImagePullSecrets))

}

func TestMergeFromSpecDeployment(t *testing.T) {
	bs := *deploymentTestBackstage.DeepCopy()
	bs.Spec.Deployment = &api.BackstageDeployment{}
	bs.Spec.Deployment.Patch = &apiextensionsv1.JSON{
		Raw: []byte(`
metadata:
  labels:
    mylabel: java
spec:
 template:
   metadata:
     labels:
       pod: backstage
   spec:
     containers:
       - name: sidecar
         image: my-image:1.0.0
       - name: backstage-backend
         resources:
           requests:
             cpu: 251m
             memory: 257Mi
     volumes:
       - ephemeral:
           volumeClaimTemplate:
             spec:
               storageClassName: "special"
         name: dynamic-plugins-root
       - emptyDir:
         name: my-vol
`),
	}

	testObj := createBackstageTest(bs).withDefaultConfig(true).
		addToDefaultConfig("deployment.yaml", "rhdh-deployment.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)

	deployment := model.GetRuntimeObject(DeploymentKey).(*BackstageDeployment)
	// label added
	assert.Equal(t, "java", deployment.deployable.GetObject().GetLabels()["mylabel"])
	assert.Equal(t, "backstage", deployment.deployable.PodObjectMeta().GetLabels()["pod"])

	// sidecar added
	assert.Equal(t, 2, len(deployment.podSpec().Containers))
	assert.Equal(t, "sidecar", deployment.podSpec().Containers[1].Name)
	assert.Equal(t, "my-image:1.0.0", deployment.podSpec().Containers[1].Image)

	// backstage container resources updated
	assert.Equal(t, "backstage-backend", deployment.container().Name)
	assert.Equal(t, "257Mi", deployment.container().Resources.Requests.Memory().String())

	// volumes
	// dynamic-plugins-root, dynamic-plugins-npmrc, dynamic-plugins-auth, my-vol
	assert.Equal(t, 4, len(deployment.podSpec().Volumes))
	assert.Equal(t, "dynamic-plugins-root", deployment.podSpec().Volumes[0].Name)
	// overrides StorageClassName
	assert.Equal(t, "special", *deployment.podSpec().Volumes[0].Ephemeral.VolumeClaimTemplate.Spec.StorageClassName)
	// adds new volume
	assert.Equal(t, "my-vol", deployment.podSpec().Volumes[3].Name)
}

// https://redhat.atlassian.net/browse/RHDHBUGS-2900
func TestInitContainerOrderInSpecDeployment(t *testing.T) {
	tests := []struct {
		name     string
		patch    string
		expected []string
	}{
		{
			name: "new init container runs before existing",
			patch: `
spec:
 template:
   spec:
     initContainers:
       - name: my-init
         image: busybox
         command: ["sh", "-c", "echo init"]
`,
			expected: []string{"my-init", "install-dynamic-plugins"},
		},
		{
			name: "new init container runs before existing by anchoring",
			patch: `
spec:
 template:
   spec:
     initContainers:
       - name: my-init
         image: busybox
         command: ["sh", "-c", "echo init"]
       - name: install-dynamic-plugins
`,
			expected: []string{"my-init", "install-dynamic-plugins"},
		},
		{
			name: "new init container runs after existing by anchoring",
			patch: `
spec:
 template:
   spec:
     initContainers:
       - name: install-dynamic-plugins
       - name: my-init
         image: busybox
         command: ["sh", "-c", "echo init"]
`,
			expected: []string{"install-dynamic-plugins", "my-init"},
		},
		{
			name: "multiple new init containers with mixed ordering",
			patch: `
spec:
 template:
   spec:
     initContainers:
       - name: pre-init
         image: busybox
         command: ["sh", "-c", "echo pre"]
       - name: install-dynamic-plugins
       - name: post-init
         image: busybox
         command: ["sh", "-c", "echo post"]
`,
			expected: []string{"pre-init", "install-dynamic-plugins", "post-init"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bs := *deploymentTestBackstage.DeepCopy()
			bs.Annotations = map[string]string{
				ListMergeAnnotation: "prepend",
			}
			bs.Spec.Deployment = &api.BackstageDeployment{}
			bs.Spec.Deployment.Patch = &apiextensionsv1.JSON{
				Raw: []byte(tt.patch),
			}

			testObj := createBackstageTest(bs).withDefaultConfig(true).
				addToDefaultConfig("deployment.yaml", "rhdh-deployment.yaml")

			model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
			assert.NoError(t, err)

			initContainers := model.backstageDeployment.podSpec().InitContainers
			assert.Equal(t, len(tt.expected), len(initContainers))
			for i, name := range tt.expected {
				assert.Equal(t, name, initContainers[i].Name)
			}
		})
	}
}

func TestImageInCRPrevailsOnEnvVar(t *testing.T) {
	bs := *deploymentTestBackstage.DeepCopy()
	bs.Spec.Deployment = &api.BackstageDeployment{}
	bs.Spec.Deployment.Patch = &apiextensionsv1.JSON{
		Raw: []byte(`
spec:
 template:
   spec:
     containers:
       - name: backstage-backend
         image: cr-image
`),
	}

	t.Setenv(BackstageImageEnvVar, "envvar-image")

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	model, err := InitObjects(context.TODO(), api.Backstage{}, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)
	// make sure env var works
	deployment := model.GetRuntimeObject(DeploymentKey).(*BackstageDeployment)
	assert.Equal(t, "envvar-image", deployment.container().Image)

	model, err = InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)
	// make sure image defined in CR overrides
	deployment = model.GetRuntimeObject(DeploymentKey).(*BackstageDeployment)
	assert.Equal(t, "cr-image", deployment.container().Image)
}

func TestFilterContainers(t *testing.T) {

	bs := *deploymentTestBackstage.DeepCopy()
	bs.Spec.Deployment = &api.BackstageDeployment{}
	bs.Spec.Deployment.Patch = &apiextensionsv1.JSON{
		Raw: []byte(`
spec:
 template:
   spec:
     containers:
       - name: c1
       - name: c2
     initContainers:
       - name: c3
`),
	}

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)
	deployment := model.GetRuntimeObject(DeploymentKey).(*BackstageDeployment)
	assert.NotNil(t, deployment)
	d := deployment

	f := containersFilter{}
	cs, err := f.getContainers(d)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(cs))
	assert.Equal(t, BackstageContainerName(), cs[0].Name)

	f = containersFilter{names: []string{}}
	cs, _ = f.getContainers(d)
	assert.Equal(t, 1, len(cs))
	assert.Equal(t, BackstageContainerName(), cs[0].Name)

	f = containersFilter{names: []string{"*"}}
	cs, _ = f.getContainers(d)
	assert.Equal(t, 4, len(cs))

	f = containersFilter{names: []string{"c123"}}
	_, err = f.getContainers(d)
	assert.Error(t, err)

	f = containersFilter{names: []string{"c1", "c2"}}
	cs, _ = f.getContainers(d)
	assert.Equal(t, 2, len(cs))
	assert.Equal(t, "c1", cs[0].Name)
	assert.NotNil(t, deployment.containerByName("c1"))

}

func TestEnvVarsWithSidecars(t *testing.T) {
	bs := *deploymentTestBackstage.DeepCopy()
	bs.Spec.Deployment = &api.BackstageDeployment{}
	bs.Spec.Deployment.Patch = &apiextensionsv1.JSON{
		Raw: []byte(`
spec:
  template:
    spec:
      containers:
        - name: sidecar
          image: busybox
`),
	}
	bs.Spec.Application.ExtraEnvs = &api.ExtraEnvs{
		Envs: []api.Env{
			{Name: "VAR1", Value: "v1", Containers: []string{"sidecar"}},
		},
	}

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)
	deployment := model.GetRuntimeObject(DeploymentKey).(*BackstageDeployment)
	assert.NotNil(t, deployment)
	sidecar := deployment.containerByName("sidecar")
	assert.NotNil(t, sidecar)
	assert.Equal(t, 1, len(sidecar.Env))
	assert.Equal(t, "VAR1", sidecar.Env[0].Name)

}

func TestDeploymentKind(t *testing.T) {

	bs := *deploymentTestBackstage.DeepCopy()
	bs.Spec.Deployment = &api.BackstageDeployment{}

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)

	deployment := model.GetRuntimeObject(DeploymentKey).(*BackstageDeployment)
	depPodSpec := deployment.podSpec()

	bs.Spec.Deployment.Kind = "StatefulSet"
	testObj = createBackstageTest(bs).withDefaultConfig(true)
	model, err = InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)

	deployment = model.GetRuntimeObject(DeploymentKey).(*BackstageDeployment)
	assert.Equal(t, "StatefulSet", deployment.deployable.GetObject().GetObjectKind().GroupVersionKind().Kind)

	ssPodSpec := deployment.podSpec()
	assert.Equal(t, depPodSpec, ssPodSpec)
}

func TestPatchedStatefulSet(t *testing.T) {
	bs := *deploymentTestBackstage.DeepCopy()
	bs.Spec.Deployment = &api.BackstageDeployment{}
	bs.Spec.Deployment.Kind = "StatefulSet"
	bs.Spec.Deployment.Patch = &apiextensionsv1.JSON{
		Raw: []byte(`
spec:
 serviceName: my-service
`),
	}
	testObj := createBackstageTest(bs).withDefaultConfig(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)

	deployment := model.GetRuntimeObject(DeploymentKey).(*BackstageDeployment)
	assert.Equal(t, "StatefulSet", deployment.deployable.GetObject().GetObjectKind().GroupVersionKind().Kind)

	ss, ok := deployment.deployable.GetObject().(*appv1.StatefulSet)
	assert.True(t, ok)
	assert.Equal(t, "my-service", ss.Spec.ServiceName)
}
