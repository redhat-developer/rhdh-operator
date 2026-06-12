package model

import (
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type DbSecretFactory struct{}

func (f DbSecretFactory) newBackstageObject() RuntimeObject {
	return &DbSecret{}
}

type DbSecret struct {
	secret *corev1.Secret
	model  *BackstageModel
}

func init() {
	registerConfig(DbSecretKey, DbSecretFactory{}, false, nil)
}

func DbSecretDefaultName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-psql-secret")
}

func (b *DbSecret) Object() runtime.Object {
	if b.secret == nil {
		return nil
	}
	return b.secret
}

// implementation of RuntimeObject interface
func (b *DbSecret) GetKey() string {
	return DbSecretKey
}

func (b *DbSecret) addToModel(model *BackstageModel, backstage api.Backstage, config runtime.Object, scheme *runtime.Scheme) error {
	b.model = model

	// Only set secret if localDb is enabled and auth secret not specified
	if !backstage.Spec.IsAuthSecretSpecified() && model.localDbEnabled && config != nil {
		b.secret = config.(*corev1.Secret)
	}

	// Always add wrapper to model (unconditional)
	model.setRuntimeObject(b)

	// Only set metadata if underlying object exists
	if b.secret != nil {
		b.setMetaInfo(backstage, scheme)
	}
	return nil
}

func (b *DbSecret) updateAndValidate(_ api.Backstage, _ *runtime.Scheme) error {

	// If no secret, nothing to do
	if b.secret == nil {
		return nil
	}

	pswd, _ := utils.GeneratePassword(24)

	dbService := b.model.GetRuntimeObject(DbServiceKey)
	if dbService == nil {
		return fmt.Errorf("database service not found in model")
	}
	service := dbService.(*DbService).service

	b.secret.StringData = map[string]string{
		"POSTGRES_PASSWORD":         pswd,
		"POSTGRESQL_ADMIN_PASSWORD": pswd,
		"POSTGRES_USER":             "postgres",
		"POSTGRES_HOST":             service.GetName(),
		"POSTGRES_PORT":             strconv.FormatInt(int64(service.Spec.Ports[0].Port), 10),
	}

	return nil
}

func (b *DbSecret) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	b.secret.SetName(DbSecretDefaultName(backstage.Name))
	setMetaInfo(b.secret, backstage, scheme)
}
