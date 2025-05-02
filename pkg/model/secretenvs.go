package model

import (
	"fmt"

	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
)

const SecretEnvsObjectKey = "secret-envs.yaml"

type SecretEnvsFactory struct{}

func (f SecretEnvsFactory) newBackstageObject() RuntimeObject {
	__sealights__.TraceFunc("e3eed41893977bf99b")
	return &SecretEnvs{}
}

type SecretEnvs struct {
	secrets *multiobject.MultiObject
}

func init() {
	__sealights__.TraceFunc("4e10b5b60ef1e01a62")
	registerConfig(SecretEnvsObjectKey, SecretEnvsFactory{}, true)
}

func addSecretEnvsFromSpec(spec bsv1.BackstageSpec, model *BackstageModel) error {
	__sealights__.TraceFunc("9980b64311122c298c")
	if spec.Application == nil || spec.Application.ExtraEnvs == nil || spec.Application.ExtraEnvs.Secrets == nil {
		return nil
	}

	for _, specSec := range spec.Application.ExtraEnvs.Secrets {
		model.backstageDeployment.addEnvVarsFrom([]string{BackstageContainerName()}, SecretObjectKind, specSec.Name, specSec.Key)
	}
	return nil
}

// implementation of RuntimeObject interface
func (p *SecretEnvs) Object() runtime.Object {
	__sealights__.TraceFunc("a89b9ed032ad530613")
	return p.secrets
}

// implementation of RuntimeObject interface
func (p *SecretEnvs) setObject(obj runtime.Object) {
	__sealights__.TraceFunc("cfd2df085967bfc600")
	p.secrets = nil
	if obj != nil {
		// p.Secret = obj.(*corev1.Secret)
		p.secrets = obj.(*multiobject.MultiObject)
	}
}

// implementation of RuntimeObject interface
func (p *SecretEnvs) EmptyObject() client.Object {
	__sealights__.TraceFunc("8e01ce285793b75d9a")
	return &corev1.Secret{}
}

// implementation of RuntimeObject interface
func (p *SecretEnvs) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	__sealights__.TraceFunc("3b24b678307758c9df")
	if p.secrets != nil {
		model.setRuntimeObject(p)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (p *SecretEnvs) updateAndValidate(m *BackstageModel, _ bsv1.Backstage) error {
	__sealights__.TraceFunc("4ecc9322ddd25d5479")

	for _, item := range p.secrets.Items {
		_, ok := item.(*corev1.Secret)
		if !ok {
			return fmt.Errorf("payload is not corev1.Secret: %T", item)
		}
		toContainers := utils.FilterContainers(m.backstageDeployment.allContainers(), item.GetAnnotations()[ContainersAnnotation])
		if toContainers == nil {
			toContainers = []string{BackstageContainerName()}
		}
		m.backstageDeployment.addEnvVarsFrom(toContainers, SecretObjectKind, item.GetName(), "")
	}

	return nil
}

// implementation of RuntimeObject interface
func (p *SecretEnvs) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	__sealights__.TraceFunc("6ac7b56a67b31c6eaf")
	for _, item := range p.secrets.Items {
		secret := item.(*corev1.Secret)
		utils.AddAnnotation(secret, ConfiguredNameAnnotation, item.GetName())
		if len(p.secrets.Items) == 1 {
			// keep for backward compatibility
			secret.Name = utils.GenerateRuntimeObjectName(backstage.Name, "backstage-envs")
		} else {
			secret.Name = fmt.Sprintf("%s-%s", utils.GenerateRuntimeObjectName(backstage.Name, "backstage-envs"), secret.Name)
		}
		setMetaInfo(secret, backstage, scheme)
	}
}
