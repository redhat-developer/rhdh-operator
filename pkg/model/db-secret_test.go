package model

import (
	"context"
	"fmt"
	"testing"

	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"
	"github.com/redhat-developer/rhdh-operator/pkg/platform"
	"k8s.io/utils/ptr"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var dbSecretBackstage = &bsv1.Backstage{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "bs",
		Namespace: "ns123",
	},
	Spec: bsv1.BackstageSpec{
		Database: &bsv1.Database{
			EnableLocalDb: ptr.To(false),
		},
	},
}

func TestEmptyDbSecret(t *testing.T) {
	__sealights__.StartTestFunc("4e3e842b2bd7031b07", t)
	defer func() { __sealights__.EndTestFunc("4e3e842b2bd7031b07", t) }()

	bs := *dbSecretBackstage.DeepCopy()

	// expected generatePassword = false (default db-secret defined) will come from preprocess
	testObj := createBackstageTest(bs).withDefaultConfig(true).withLocalDb().addToDefaultConfig("db-secret.yaml", "db-empty-secret.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model.LocalDbSecret)
	assert.Equal(t, fmt.Sprintf("backstage-psql-secret-%s", bs.Name), model.LocalDbSecret.secret.Name)

	dbss := model.localDbStatefulSet
	assert.NotNil(t, dbss)
	assert.Equal(t, 1, len(dbss.container().EnvFrom))

	assert.Equal(t, model.LocalDbSecret.secret.Name, dbss.container().EnvFrom[0].SecretRef.Name)
}

func TestDefaultWithGeneratedSecrets(t *testing.T) {
	__sealights__.StartTestFunc("64b5fc1f71cd9287bc", t)
	defer func() { __sealights__.EndTestFunc("64b5fc1f71cd9287bc", t) }()
	bs := *dbSecretBackstage.DeepCopy()

	// expected generatePassword = true (no db-secret defined) will come from preprocess
	testObj := createBackstageTest(bs).withDefaultConfig(true).withLocalDb().addToDefaultConfig("db-secret.yaml", "db-generated-secret.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("backstage-psql-secret-%s", bs.Name), model.LocalDbSecret.secret.Name)
	//should be generated
	//	assert.NotEmpty(t, model.LocalDbSecret.secret.StringData["POSTGRES_USER"])
	//	assert.NotEmpty(t, model.LocalDbSecret.secret.StringData["POSTGRES_PASSWORD"])

	dbss := model.localDbStatefulSet
	assert.NotNil(t, dbss)
	assert.Equal(t, 1, len(dbss.container().EnvFrom))
	assert.Equal(t, model.LocalDbSecret.secret.Name, dbss.container().EnvFrom[0].SecretRef.Name)
}

func TestSpecifiedSecret(t *testing.T) {
	__sealights__.StartTestFunc("06315ca8ac8f34e552", t)
	defer func() { __sealights__.EndTestFunc("06315ca8ac8f34e552", t) }()
	bs := *dbSecretBackstage.DeepCopy()
	bs.Spec.Database.AuthSecretName = "custom-db-secret"

	// expected generatePassword = false (db-secret defined in the spec) will come from preprocess
	testObj := createBackstageTest(bs).withDefaultConfig(true).withLocalDb().addToDefaultConfig("db-secret.yaml", "db-generated-secret.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.Nil(t, model.LocalDbSecret)

	assert.Equal(t, bs.Spec.Database.AuthSecretName, model.localDbStatefulSet.container().EnvFrom[0].SecretRef.Name)
	assert.Equal(t, bs.Spec.Database.AuthSecretName, model.backstageDeployment.container().EnvFrom[0].SecretRef.Name)
}
