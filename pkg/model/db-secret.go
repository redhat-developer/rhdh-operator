package model

import (
	"strconv"

	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type DbSecretFactory struct{}

func (f DbSecretFactory) newBackstageObject() RuntimeObject {
	__sealights__.TraceFunc("1cf67ef93b2173f04d")
	return &DbSecret{}
}

type DbSecret struct {
	secret *corev1.Secret
}

func init() {
	__sealights__.TraceFunc("5fe95517bf28559bbf")
	registerConfig("db-secret.yaml", DbSecretFactory{}, false)
}

func DbSecretDefaultName(backstageName string) string {
	__sealights__.TraceFunc("560749b6d30827796b")
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-psql-secret")
}

// implementation of RuntimeObject interface
func (b *DbSecret) Object() runtime.Object {
	__sealights__.TraceFunc("c4a614fff70eb71129")
	return b.secret
}

// implementation of RuntimeObject interface
func (b *DbSecret) setObject(obj runtime.Object) {
	__sealights__.TraceFunc("c20dbe8571ea7a6f15")
	b.secret = nil
	if obj != nil {
		b.secret = obj.(*corev1.Secret)
	}
}

// implementation of RuntimeObject interface
func (b *DbSecret) addToModel(model *BackstageModel, backstage bsv1.Backstage) (bool, error) {
	__sealights__.TraceFunc("ddb744592a274bf1b2")

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
	__sealights__.TraceFunc("e1214468897486ab56")
	return &corev1.Secret{}
}

// implementation of RuntimeObject interface
func (b *DbSecret) updateAndValidate(model *BackstageModel, backstage bsv1.Backstage) error {
	__sealights__.TraceFunc("9f4ee346e44d0ebdd7")

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
	__sealights__.TraceFunc("1d6060b460cc29c69b")
	b.secret.SetName(DbSecretDefaultName(backstage.Name))
	setMetaInfo(b.secret, backstage, scheme)
}
