package model

import (
	"fmt"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"

	"k8s.io/apimachinery/pkg/runtime"

	corev1 "k8s.io/api/core/v1"
)

type ConfigMapEnvsFactory struct{}

func (f ConfigMapEnvsFactory) newBackstageObject() RuntimeObject {
	return &ConfigMapEnvs{ConfigMaps: &multiobject.MultiObject{}}
}

type ConfigMapEnvs struct {
	ConfigMaps *multiobject.MultiObject
	model      *BackstageModel
}

func init() {
	registerConfig("configmap-envs.yaml", ConfigMapEnvsFactory{}, true, mergeMultiObjectConfigs)
}

func (p *ConfigMapEnvs) addExternalConfig(spec api.BackstageSpec) error {
	if spec.Application == nil || spec.Application.ExtraEnvs == nil || spec.Application.ExtraEnvs.ConfigMaps == nil {
		return nil
	}

	for _, specCm := range spec.Application.ExtraEnvs.ConfigMaps {
		err := p.model.backstageDeployment.addEnvVarsFrom(containersFilter{names: specCm.Containers}, ConfigMapObjectKind, specCm.Name, specCm.Key)
		if err != nil {
			return fmt.Errorf("failed to add env vars on config map %s: %w", specCm.Name, err)
		}
	}
	return nil
}

// Object implements RuntimeObject interface
func (p *ConfigMapEnvs) Object() runtime.Object {
	return p.ConfigMaps
}

func (p *ConfigMapEnvs) setObject(obj runtime.Object) {
	p.ConfigMaps = nil
	if obj != nil {
		p.ConfigMaps = obj.(*multiobject.MultiObject)
	}
}

// implementation of RuntimeObject interface
func (p *ConfigMapEnvs) addToModel(model *BackstageModel, backstage api.Backstage) (bool, error) {
	p.model = model
	if p.ConfigMaps != nil && len(p.ConfigMaps.Items) > 0 {
		model.setRuntimeObject(p)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapEnvs) updateAndValidate(backstage api.Backstage) error {
	for _, item := range p.ConfigMaps.Items {
		cm, ok := item.(*corev1.ConfigMap)
		if !ok {
			return fmt.Errorf("expected ConfigMap, got %T", item)
		}
		err := p.model.backstageDeployment.addEnvVarsFrom(containersFilter{annotation: cm.GetAnnotations()[ContainersAnnotation]}, ConfigMapObjectKind,
			cm.Name, "")
		if err != nil {
			return fmt.Errorf("failed to add env vars on configmap %s: %w", cm.Name, err)
		}
	}
	return nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapEnvs) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	//for _, item := range p.ConfigMaps.Items {
	//	cm := item.(*corev1.ConfigMap)
	//	cm.Name = ConfigMapEnvsDefaultName(backstage.Name, cm.Annotations[SourceAnnotation])
	//	setMetaInfo(cm, backstage, scheme)
	//}
	setMultiObjectConfigMetaInfo(p.ConfigMaps, "envs", backstage, scheme)
}
