package model

import (
	"fmt"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
)

type SecretEnvsFactory struct{}

func (f SecretEnvsFactory) newBackstageObject() RuntimeObject {
	return &SecretEnvs{}
}

type SecretEnvs struct {
	secrets *multiobject.MultiObject
	model   *BackstageModel
}

func init() {
	registerConfig(SecretEnvsKey, SecretEnvsFactory{}, true, nil)
}

func (p *SecretEnvs) Object() runtime.Object {
	if p.secrets != nil && len(p.secrets.Items) > 0 {
		return p.secrets
	}
	return nil
}

// implementation of RuntimeObject interface
func (p *SecretEnvs) GetKey() string {
	return SecretEnvsKey
}

func (p *SecretEnvs) addToModel(model *BackstageModel, backstage api.Backstage, config runtime.Object, scheme *runtime.Scheme) error {
	p.model = model
	if config != nil {
		p.secrets = config.(*multiobject.MultiObject)
	} else {
		// Create empty secrets - might be populated later from spec
		p.secrets = &multiobject.MultiObject{Items: []client.Object{}}
	}

	// Always add to model so updateAndValidate is called (may process spec secrets)
	model.setRuntimeObject(p)
	if p.secrets != nil && len(p.secrets.Items) > 0 {
		p.setMetaInfo(backstage, scheme)
	}
	return nil
}

func (p *SecretEnvs) updateAndValidate(backstage api.Backstage, scheme *runtime.Scheme) error {
	deployment := p.model.getDeployment()
	if deployment == nil {
		return fmt.Errorf("backstage deployment not found in model")
	}

	// Process secrets from config files
	if p.secrets != nil {
		for _, item := range p.secrets.Items {
			_, ok := item.(*corev1.Secret)
			if !ok {
				return fmt.Errorf("payload is not Secret kind: %T", item)
			}
			err := deployment.addEnvVarsFrom(containersFilter{annotation: item.GetAnnotations()[ContainersAnnotation]}, SecretObjectKind, item.GetName(), "")
			if err != nil {
				return fmt.Errorf("failed to add env vars from secret %s: %w", item.GetName(), err)
			}
		}
	}

	// Process secrets from CR spec (formerly addExternalConfig)
	if backstage.Spec.Application != nil && backstage.Spec.Application.ExtraEnvs != nil && backstage.Spec.Application.ExtraEnvs.Secrets != nil {
		for _, specSec := range backstage.Spec.Application.ExtraEnvs.Secrets {
			err := deployment.addEnvVarsFrom(containersFilter{names: specSec.Containers}, SecretObjectKind, specSec.Name, specSec.Key)
			if err != nil {
				return fmt.Errorf("failed to add env vars on secret %s: %w", specSec.Name, err)
			}
		}
	}

	return nil
}

func (p *SecretEnvs) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	setMultiObjectConfigMetaInfo(p.secrets, "envs", backstage, scheme)
}
