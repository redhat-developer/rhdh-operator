package model

import (
	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	corev1 "k8s.io/api/core/v1"
)

type SecretEnvsFactory struct{}

func (f SecretEnvsFactory) newBackstageObject() RuntimeObject {
	return &SecretEnvs{}
}

type SecretEnvs struct {
	Secret *corev1.Secret
}

func init() {
	registerConfig("secret-envs.yaml", SecretEnvsFactory{}, false)
}

// implementation of RuntimeObject interface
func (p *SecretEnvs) Object() runtime.Object {
	return p.Secret
}

func addSecretEnvsFromSpec(spec bsv1.BackstageSpec, model *BackstageModel) error {
	if spec.Application == nil || spec.Application.ExtraEnvs == nil || spec.Application.ExtraEnvs.Secrets == nil {
		return nil
	}

	for _, specSec := range spec.Application.ExtraEnvs.Secrets {
		utils.AddEnvVarsFrom(model.backstageDeployment.container(), utils.SecretObjectKind, specSec.Name, specSec.Key)
	}
	return nil
}

func (p *SecretEnvs) setObject(obj runtime.Object) {
	p.Secret = nil
	if obj != nil {
		p.Secret = obj.(*corev1.Secret)
	}
}

// implementation of RuntimeObject interface
func (p *SecretEnvs) EmptyObject() client.Object {
	return &corev1.Secret{}
}

// implementation of RuntimeObject interface
func (p *SecretEnvs) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	if p.Secret != nil {
		model.setRuntimeObject(p)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (p *SecretEnvs) updateAndValidate(m *BackstageModel, _ bsv1.Backstage) error {
	utils.AddEnvVarsFrom(m.backstageDeployment.container(), utils.SecretObjectKind,
		p.Secret.Name, "")
	return nil
}

// implementation of RuntimeObject interface
func (p *SecretEnvs) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	p.Secret.SetName(utils.GenerateRuntimeObjectName(backstage.Name, "backstage-envs"))
	setMetaInfo(p.Secret, backstage, scheme)
}
