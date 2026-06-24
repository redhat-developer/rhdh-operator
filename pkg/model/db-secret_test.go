package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/redhat-developer/rhdh-operator/pkg/platform"

	"k8s.io/utils/ptr"

	"github.com/redhat-developer/rhdh-operator/api"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

var dbSecretBackstage = &api.Backstage{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "bs",
		Namespace: "ns123",
	},
	Spec: api.BackstageSpec{
		Database: &api.Database{
			EnableLocalDb: ptr.To(false),
		},
	},
}

func TestEmptyDbSecret(t *testing.T) {

	bs := *dbSecretBackstage.DeepCopy()

	// expected generatePassword = false (default db-secret defined) will come from preprocess
	testObj := createBackstageTest(bs).withDefaultConfig(true).withLocalDb(true).addToDefaultConfig("db-secret.yaml", "db-empty-secret.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model.GetRuntimeObject(DbSecretKey).(*DbSecret))
	assert.Equal(t, fmt.Sprintf("backstage-psql-secret-%s", bs.Name), model.GetRuntimeObject(DbSecretKey).(*DbSecret).secret.Name)

	dbss := model.GetRuntimeObject(DbStatefulSetKey).(*DbStatefulSet)
	assert.NotNil(t, dbss)
	assert.Equal(t, 1, len(dbss.container().EnvFrom))

	assert.Equal(t, model.GetRuntimeObject(DbSecretKey).(*DbSecret).secret.Name, dbss.container().EnvFrom[0].SecretRef.Name)
}

func TestDefaultWithGeneratedSecrets(t *testing.T) {
	bs := *dbSecretBackstage.DeepCopy()

	// expected generatePassword = true (no db-secret defined) will come from preprocess
	testObj := createBackstageTest(bs).withDefaultConfig(true).withLocalDb(true).addToDefaultConfig("db-secret.yaml", "db-generated-secret.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("backstage-psql-secret-%s", bs.Name), model.GetRuntimeObject(DbSecretKey).(*DbSecret).secret.Name)
	//should be generated
	//	assert.NotEmpty(t, model.GetRuntimeObject(DbSecretKey).(*DbSecret).secret.StringData["POSTGRES_USER"])
	//	assert.NotEmpty(t, model.GetRuntimeObject(DbSecretKey).(*DbSecret).secret.StringData["POSTGRES_PASSWORD"])

	dbss := model.GetRuntimeObject(DbStatefulSetKey).(*DbStatefulSet)
	assert.NotNil(t, dbss)
	assert.Equal(t, 1, len(dbss.container().EnvFrom))
	assert.Equal(t, model.GetRuntimeObject(DbSecretKey).(*DbSecret).secret.Name, dbss.container().EnvFrom[0].SecretRef.Name)
}

func TestSpecifiedSecret(t *testing.T) {
	bs := *dbSecretBackstage.DeepCopy()
	bs.Spec.Database.AuthSecretName = "custom-db-secret"

	// expected generatePassword = false (db-secret defined in the spec) will come from preprocess
	testObj := createBackstageTest(bs).withDefaultConfig(true).withLocalDb(true).addToDefaultConfig("db-secret.yaml", "db-generated-secret.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	// When custom auth secret is specified, db-secret should not be applied
	dbSecret := model.GetRuntimeObject(DbSecretKey)
	assert.Nil(t, dbSecret, "DbSecret should not be returned when custom auth secret is specified")

	assert.Equal(t, bs.Spec.Database.AuthSecretName, model.GetRuntimeObject(DbStatefulSetKey).(*DbStatefulSet).container().EnvFrom[0].SecretRef.Name)
	assert.Equal(t, bs.Spec.Database.AuthSecretName, model.GetRuntimeObject(DeploymentKey).(*BackstageDeployment).container().EnvFrom[0].SecretRef.Name)
}
