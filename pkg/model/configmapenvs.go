package model

import (
	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha4"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	corev1 "k8s.io/api/core/v1"
)

type ConfigMapEnvsFactory struct{}

func (f ConfigMapEnvsFactory) newBackstageObject() RuntimeObject {
	return &ConfigMapEnvs{}
}

type ConfigMapEnvs struct {
	ConfigMap *corev1.ConfigMap
}

func init() {
	registerConfig("configmap-envs.yaml", ConfigMapEnvsFactory{}, false)
}

func addConfigMapEnvsFromSpec(spec bsv1.BackstageSpec, model *BackstageModel) {
	if spec.Application == nil || spec.Application.ExtraEnvs == nil || spec.Application.ExtraEnvs.ConfigMaps == nil {
		return
	}

	for _, specCm := range spec.Application.ExtraEnvs.ConfigMaps {
		model.backstageDeployment.addEnvVarsFrom([]string{BackstageContainerName()}, ConfigMapObjectKind, specCm.Name, specCm.Key)
	}
}

// Object implements RuntimeObject interface
func (p *ConfigMapEnvs) Object() runtime.Object {
	return p.ConfigMap
}

func (p *ConfigMapEnvs) setObject(obj runtime.Object) {
	p.ConfigMap = nil
	if obj != nil {
		p.ConfigMap = obj.(*corev1.ConfigMap)
	}
}

// EmptyObject implements RuntimeObject interface
func (p *ConfigMapEnvs) EmptyObject() client.Object {
	return &corev1.ConfigMap{}
}

// implementation of RuntimeObject interface
func (p *ConfigMapEnvs) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	if p.ConfigMap != nil {
		model.setRuntimeObject(p)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapEnvs) updateAndValidate(m *BackstageModel, _ bsv1.Backstage) error {
	m.backstageDeployment.addEnvVarsFrom([]string{BackstageContainerName()}, ConfigMapObjectKind,
		p.ConfigMap.Name, "")
	return nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapEnvs) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	p.ConfigMap.SetName(utils.GenerateRuntimeObjectName(backstage.Name, "backstage-envs"))
	setMetaInfo(p.ConfigMap, backstage, scheme)
}
