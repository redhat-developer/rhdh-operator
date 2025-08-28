package model

import (
	"fmt"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha4"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	corev1 "k8s.io/api/core/v1"
)

const SecretEnvsObjectKey = "secret-envs.yaml"

type SecretEnvsFactory struct{}

func (f SecretEnvsFactory) newBackstageObject() RuntimeObject {
	return &SecretEnvs{}
}

type SecretEnvs struct {
	secrets *multiobject.MultiObject
	model   *BackstageModel
}

func init() {
	registerConfig(SecretEnvsObjectKey, SecretEnvsFactory{}, true)
}

func (p *SecretEnvs) addExternalConfig(spec bsv1.BackstageSpec) error {
	if spec.Application == nil || spec.Application.ExtraEnvs == nil || spec.Application.ExtraEnvs.Secrets == nil {
		return nil
	}

	for _, specSec := range spec.Application.ExtraEnvs.Secrets {
		err := p.model.backstageDeployment.addEnvVarsFrom(containersFilter{names: specSec.Containers}, SecretObjectKind, specSec.Name, specSec.Key)
		if err != nil {
			return fmt.Errorf("failed to add env vars on secret %s: %w", specSec.Name, err)
		}
	}
	return nil
}

// implementation of RuntimeObject interface
func (p *SecretEnvs) Object() runtime.Object {
	return p.secrets
}

// implementation of RuntimeObject interface
func (p *SecretEnvs) setObject(obj runtime.Object) {
	p.secrets = nil
	if obj != nil {
		p.secrets = obj.(*multiobject.MultiObject)
	}
}

// implementation of RuntimeObject interface
func (p *SecretEnvs) EmptyObject() client.Object {
	return &corev1.Secret{}
}

// implementation of RuntimeObject interface
func (p *SecretEnvs) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	p.model = model
	if p.secrets != nil {
		model.setRuntimeObject(p)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (p *SecretEnvs) updateAndValidate(_ bsv1.Backstage) error {

	for _, item := range p.secrets.Items {
		_, ok := item.(*corev1.Secret)
		if !ok {
			return fmt.Errorf("payload is not Secret kind: %T", item)
		}
		err := p.model.backstageDeployment.addEnvVarsFrom(containersFilter{annotation: item.GetAnnotations()[ContainersAnnotation]}, SecretObjectKind, item.GetName(), "")
		if err != nil {
			return fmt.Errorf("failed to add env vars from secret %s: %w", item.GetName(), err)
		}
	}

	return nil
}

// implementation of RuntimeObject interface
func (p *SecretEnvs) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
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
