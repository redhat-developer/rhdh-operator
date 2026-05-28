package model

import (
	"fmt"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type ConfigMapEnvsFactory struct{}

func (f ConfigMapEnvsFactory) newBackstageObject() RuntimeObject {
	return &ConfigMapEnvs{}
}

type ConfigMapEnvs struct {
	ConfigMaps *multiobject.MultiObject
	model      *BackstageModel
}

func init() {
	registerConfig(ConfigMapEnvsKey, ConfigMapEnvsFactory{}, true, mergeMultiObjectConfigs)
}

func (p *ConfigMapEnvs) Object() runtime.Object {
	if p.ConfigMaps != nil && len(p.ConfigMaps.Items) > 0 {
		return p.ConfigMaps
	}
	return nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapEnvs) GetKey() string {
	return ConfigMapEnvsKey
}

func (p *ConfigMapEnvs) addToModel(model *BackstageModel, backstage api.Backstage, config runtime.Object, scheme *runtime.Scheme) error {
	p.model = model
	if config != nil {
		p.ConfigMaps = config.(*multiobject.MultiObject)
	}

	// Always add to model so updateAndValidate is called (may process spec ConfigMaps)
	model.setRuntimeObject(p)
	if p.ConfigMaps != nil && len(p.ConfigMaps.Items) > 0 {
		p.setMetaInfo(backstage, scheme)
	}
	return nil
}

func (p *ConfigMapEnvs) updateAndValidate(backstage api.Backstage, scheme *runtime.Scheme) error {
	deployment := p.model.getDeployment()
	if deployment == nil {
		return fmt.Errorf("backstage deployment not found in model")
	}

	// Process configmaps from config files
	if p.ConfigMaps != nil {
		for _, item := range p.ConfigMaps.Items {
			cm, ok := item.(*corev1.ConfigMap)
			if !ok {
				return fmt.Errorf("payload is not ConfigMap kind: %T", item)
			}
			err := deployment.addEnvVarsFrom(containersFilter{annotation: cm.GetAnnotations()[ContainersAnnotation]}, ConfigMapObjectKind, cm.Name, "")
			if err != nil {
				return fmt.Errorf("failed to add env vars from configmap %s: %w", cm.Name, err)
			}
		}
	}

	// Process configmaps from CR spec (formerly addExternalConfig)
	if backstage.Spec.Application != nil && backstage.Spec.Application.ExtraEnvs != nil && backstage.Spec.Application.ExtraEnvs.ConfigMaps != nil {
		for _, specCm := range backstage.Spec.Application.ExtraEnvs.ConfigMaps {
			err := deployment.addEnvVarsFrom(containersFilter{names: specCm.Containers}, ConfigMapObjectKind, specCm.Name, specCm.Key)
			if err != nil {
				return fmt.Errorf("failed to add env vars on config map %s: %w", specCm.Name, err)
			}
		}
	}

	return nil
}

func (p *ConfigMapEnvs) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	setMultiObjectConfigMetaInfo(p.ConfigMaps, "envs", backstage, scheme)
}
