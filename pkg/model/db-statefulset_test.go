package model

import (
	"context"
	"os"
	"testing"

	"github.com/redhat-developer/rhdh-operator/pkg/platform"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/utils/ptr"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

var dbStatefulSetBackstage = &bsv1.Backstage{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "bs",
		Namespace: "ns123",
	},
	Spec: bsv1.BackstageSpec{
		Database:    &bsv1.Database{},
		Application: &bsv1.Application{},
	},
}

// test default StatefulSet
func TestDefault(t *testing.T) {
	bs := *dbStatefulSetBackstage.DeepCopy()
	testObj := createBackstageTest(bs).withDefaultConfig(true)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)

	assert.Equal(t, model.LocalDbService.service.Name, model.localDbStatefulSet.statefulSet.Spec.ServiceName)
	assert.Equal(t, corev1.ClusterIPNone, model.LocalDbService.service.Spec.ClusterIP)
}

// It tests the overriding image feature
func TestOverrideDbImage(t *testing.T) {
	bs := *dbStatefulSetBackstage.DeepCopy()

	bs.Spec.Database.EnableLocalDb = ptr.To(false)

	testObj := createBackstageTest(bs).withDefaultConfig(true).
		addToDefaultConfig("db-statefulset.yaml", "janus-db-statefulset.yaml").withLocalDb()

	_ = os.Setenv(LocalDbImageEnvVar, "dummy")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)

	assert.Equal(t, "dummy", model.localDbStatefulSet.statefulSet.Spec.Template.Spec.Containers[0].Image)
}

// test bs.Spec.Application.ImagePullSecrets shared with StatefulSet
func TestImagePullSecretSpec(t *testing.T) {
	bs := *dbStatefulSetBackstage.DeepCopy()
	bs.Spec.Application.ImagePullSecrets = []string{"my-secret1", "my-secret2"}

	testObj := createBackstageTest(bs).withDefaultConfig(true)
	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)
	assert.NoError(t, err)

	assert.Equal(t, 2, len(model.localDbStatefulSet.statefulSet.Spec.Template.Spec.ImagePullSecrets))
	assert.Equal(t, "my-secret1", model.localDbStatefulSet.statefulSet.Spec.Template.Spec.ImagePullSecrets[0].Name)
	assert.Equal(t, "my-secret2", model.localDbStatefulSet.statefulSet.Spec.Template.Spec.ImagePullSecrets[1].Name)

	// no image pull secrets specified
	bs = *dbStatefulSetBackstage.DeepCopy()
	testObj = createBackstageTest(bs).withDefaultConfig(true).
		addToDefaultConfig("db-statefulset.yaml", "ips-db-statefulset.yaml")

	model, err = InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	if assert.NoError(t, err) {
		// if imagepullsecrets not defined - default used
		assert.Equal(t, 2, len(model.localDbStatefulSet.statefulSet.Spec.Template.Spec.ImagePullSecrets))
		assert.Equal(t, "ips-db1", model.localDbStatefulSet.statefulSet.Spec.Template.Spec.ImagePullSecrets[0].Name)
		assert.Equal(t, "ips-db2", model.localDbStatefulSet.statefulSet.Spec.Template.Spec.ImagePullSecrets[1].Name)
	}

	// empty list of image pull secrets
	bs = *dbStatefulSetBackstage.DeepCopy()
	bs.Spec.Application.ImagePullSecrets = []string{}

	testObj = createBackstageTest(bs).withDefaultConfig(true).
		addToDefaultConfig("db-statefulset.yaml", "ips-db-statefulset.yaml")

	model, err = InitObjects(context.TODO(), bs, testObj.externalConfig, platform.OpenShift, testObj.scheme)
	if assert.NoError(t, err) {
		assert.Equal(t, 0, len(model.localDbStatefulSet.statefulSet.Spec.Template.Spec.ImagePullSecrets))
	}
}
