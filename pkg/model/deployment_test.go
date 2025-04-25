package model

import (
	"context"
	"testing"

	"github.com/redhat-developer/rhdh-operator/pkg/platform"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"k8s.io/utils/ptr"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

var deploymentTestBackstage = bsv1.Backstage{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "bs",
		Namespace: "ns123",
	},
	Spec: bsv1.BackstageSpec{
		Database: &bsv1.Database{
			EnableLocalDb: ptr.To(false),
		},
		Application: &bsv1.Application{},
	},
}

func TestSpecs(t *testing.T) {
	bs := *deploymentTestBackstage.DeepCopy()
	bs.Spec.Application.Image = ptr.To("my-image:1.0.0")
	bs.Spec.Application.Replicas = ptr.To(int32(3))
	bs.Spec.Application.ImagePullSecrets = []string{"my-secret"}

	testObj := createBackstageTest(bs).withDefaultConfig(true).
		addToDefaultConfig("deployment.yaml", "janus-deployment.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)

	assert.Equal(t, "my-image:1.0.0", model.backstageDeployment.container().Image)
	assert.Equal(t, int32(3), *model.backstageDeployment.deployment.Spec.Replicas)
	assert.Equal(t, 1, len(model.backstageDeployment.deployment.Spec.Template.Spec.ImagePullSecrets))
	assert.Equal(t, "my-secret", model.backstageDeployment.deployment.Spec.Template.Spec.ImagePullSecrets[0].Name)

}

func TestWorkingDirMount(t *testing.T) {
	bs := *deploymentTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).
		addToDefaultConfig("deployment.yaml", "working-dir-mount.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)

	assert.Equal(t, "/my/home", model.backstageDeployment.defaultMountPath())
	fileor := bsv1.FileObjectRef{
		Name:      "test",
		MountPath: "subpath",
	}
	mp, sp := model.backstageDeployment.mountPath(fileor.MountPath, "", "")
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

	assert.Equal(t, 2, len(model.backstageDeployment.podSpec().Containers))
	assert.Equal(t, "dummy", model.backstageDeployment.container().Image)
	assert.Equal(t, "dummy", model.backstageDeployment.podSpec().InitContainers[0].Image)
	assert.Equal(t, "busybox", model.backstageDeployment.podSpec().Containers[1].Image)

}

func TestSpecImagePullSecrets(t *testing.T) {
	bs := *deploymentTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).
		addToDefaultConfig("deployment.yaml", "ips-deployment.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)

	// if imagepullsecrets not defined - default used
	assert.Equal(t, 2, len(model.backstageDeployment.deployment.Spec.Template.Spec.ImagePullSecrets))
	assert.Equal(t, "ips1", model.backstageDeployment.deployment.Spec.Template.Spec.ImagePullSecrets[0].Name)

	bs.Spec.Application.ImagePullSecrets = []string{}

	testObj = createBackstageTest(bs).withDefaultConfig(true).
		addToDefaultConfig("deployment.yaml", "ips-deployment.yaml")

	model, err = InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)

	// if explicitly set empty slice - they are empty
	assert.Equal(t, 0, len(model.backstageDeployment.deployment.Spec.Template.Spec.ImagePullSecrets))

}

func TestMergeFromSpecDeployment(t *testing.T) {
	bs := *deploymentTestBackstage.DeepCopy()
	bs.Spec.Deployment = &bsv1.BackstageDeployment{}
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
		addToDefaultConfig("deployment.yaml", "janus-deployment.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)

	// label added
	assert.Equal(t, "java", model.backstageDeployment.deployment.Labels["mylabel"])
	assert.Equal(t, "backstage", model.backstageDeployment.deployment.Spec.Template.Labels["pod"])

	// sidecar added
	assert.Equal(t, 2, len(model.backstageDeployment.deployment.Spec.Template.Spec.Containers))
	assert.Equal(t, "sidecar", model.backstageDeployment.deployment.Spec.Template.Spec.Containers[1].Name)
	assert.Equal(t, "my-image:1.0.0", model.backstageDeployment.deployment.Spec.Template.Spec.Containers[1].Image)

	// backstage container resources updated
	assert.Equal(t, "backstage-backend", model.backstageDeployment.container().Name)
	assert.Equal(t, "257Mi", model.backstageDeployment.container().Resources.Requests.Memory().String())

	// volumes
	// dynamic-plugins-root, dynamic-plugins-npmrc, dynamic-plugins-auth, my-vol
	assert.Equal(t, 4, len(model.backstageDeployment.deployment.Spec.Template.Spec.Volumes))
	assert.Equal(t, "dynamic-plugins-root", model.backstageDeployment.deployment.Spec.Template.Spec.Volumes[0].Name)
	// overrides StorageClassName
	assert.Equal(t, "special", *model.backstageDeployment.deployment.Spec.Template.Spec.Volumes[0].Ephemeral.VolumeClaimTemplate.Spec.StorageClassName)
	// adds new volume
	assert.Equal(t, "my-vol", model.backstageDeployment.deployment.Spec.Template.Spec.Volumes[3].Name)
}

func TestImageInCRPrevailsOnEnvVar(t *testing.T) {
	bs := *deploymentTestBackstage.DeepCopy()
	bs.Spec.Deployment = &bsv1.BackstageDeployment{}
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

	model, err := InitObjects(context.TODO(), bsv1.Backstage{}, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)
	// make sure env var works
	assert.Equal(t, "envvar-image", model.backstageDeployment.container().Image)

	model, err = InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)
	// make sure image defined in CR overrides
	assert.Equal(t, "cr-image", model.backstageDeployment.container().Image)
}

// to remove when stop supporting v1alpha1
func TestDeploymentFieldPrevailsOnDeprecated(t *testing.T) {
	bs := *deploymentTestBackstage.DeepCopy()
	bs.Spec.Application.Image = ptr.To("app-image")
	bs.Spec.Application.Replicas = ptr.To(int32(2))
	bs.Spec.Deployment = &bsv1.BackstageDeployment{}
	bs.Spec.Deployment.Patch = &apiextensionsv1.JSON{
		Raw: []byte(`
spec:
 replicas: 3
 template:
   spec:
     containers:
       - name: backstage-backend
         image: deployment-image
`),
	}

	testObj := createBackstageTest(bs).withDefaultConfig(true).
		addToDefaultConfig("deployment.yaml", "janus-deployment.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	assert.NoError(t, err)

	assert.Equal(t, "backstage-backend", model.backstageDeployment.container().Name)
	assert.Equal(t, "deployment-image", model.backstageDeployment.container().Image)
	assert.Equal(t, int32(3), *model.backstageDeployment.deployment.Spec.Replicas)
}
