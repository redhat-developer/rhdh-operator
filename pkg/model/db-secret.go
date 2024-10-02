package model

import (
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	bsv1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha2"
	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type DbSecretFactory struct{}

func (f DbSecretFactory) newBackstageObject() RuntimeObject {
	return &DbSecret{}
}

type DbSecret struct {
	secret *corev1.Secret
}

func init() {
	registerConfig("db-secret.yaml", DbSecretFactory{}, false)
}

func DbSecretDefaultName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-psql-secret")
}

// implementation of RuntimeObject interface
func (b *DbSecret) Object() runtime.Object {
	return b.secret
}

// implementation of RuntimeObject interface
func (b *DbSecret) setObject(obj runtime.Object) {
	b.secret = nil
	if obj != nil {
		b.secret = obj.(*corev1.Secret)
	}
}

// implementation of RuntimeObject interface
func (b *DbSecret) addToModel(model *BackstageModel, backstage bsv1.Backstage) (bool, error) {

	// do not add if specified
	if backstage.Spec.IsAuthSecretSpecified() {
		return false, nil
	}

	if b.secret != nil && model.localDbEnabled {
		model.setRuntimeObject(b)
		model.LocalDbSecret = b
		return true, nil
	}

	return false, nil
}

// implementation of RuntimeObject interface
func (b *DbSecret) EmptyObject() client.Object {
	return &corev1.Secret{}
}

// implementation of RuntimeObject interface
func (b *DbSecret) validate(model *BackstageModel, backstage bsv1.Backstage) error {

	pswd, _ := utils.GeneratePassword(24)

	service := model.LocalDbService

	b.secret.StringData = map[string]string{
		"POSTGRES_PASSWORD":         pswd,
		"POSTGRESQL_ADMIN_PASSWORD": pswd,
		"POSTGRES_USER":             "postgres",
		"POSTGRES_HOST":             service.service.GetName(),
		"POSTGRES_PORT":             strconv.FormatInt(int64(service.service.Spec.Ports[0].Port), 10),
	}

	return nil
}

func (b *DbSecret) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	b.secret.SetName(DbSecretDefaultName(backstage.Name))
	setMetaInfo(b.secret, backstage, scheme)
}
