package model

import (
	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
)

type ConfigMapEnvsFactory struct{}

func (f ConfigMapEnvsFactory) newBackstageObject() RuntimeObject {
	__sealights__.TraceFunc("f362c98abbe65af043")
	return &ConfigMapEnvs{}
}

type ConfigMapEnvs struct {
	ConfigMap *corev1.ConfigMap
}

func init() {
	__sealights__.TraceFunc("abf3e7c7357948aa05")
	registerConfig("configmap-envs.yaml", ConfigMapEnvsFactory{}, false)
}

func addConfigMapEnvsFromSpec(spec bsv1.BackstageSpec, model *BackstageModel) {
	__sealights__.TraceFunc("fa0ea0cc1bcc9e1450")
	if spec.Application == nil || spec.Application.ExtraEnvs == nil || spec.Application.ExtraEnvs.ConfigMaps == nil {
		return
	}

	for _, specCm := range spec.Application.ExtraEnvs.ConfigMaps {
		model.backstageDeployment.addEnvVarsFrom([]string{BackstageContainerName()}, ConfigMapObjectKind, specCm.Name, specCm.Key)
	}
}

// Object implements RuntimeObject interface
func (p *ConfigMapEnvs) Object() runtime.Object {
	__sealights__.TraceFunc("56a5f2135b55c48bf2")
	return p.ConfigMap
}

func (p *ConfigMapEnvs) setObject(obj runtime.Object) {
	__sealights__.TraceFunc("ecb2fedb500ac4b508")
	p.ConfigMap = nil
	if obj != nil {
		p.ConfigMap = obj.(*corev1.ConfigMap)
	}
}

// EmptyObject implements RuntimeObject interface
func (p *ConfigMapEnvs) EmptyObject() client.Object {
	__sealights__.TraceFunc("1166b990bfcdb9d394")
	return &corev1.ConfigMap{}
}

// implementation of RuntimeObject interface
func (p *ConfigMapEnvs) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	__sealights__.TraceFunc("db7f1618209271757a")
	if p.ConfigMap != nil {
		model.setRuntimeObject(p)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapEnvs) updateAndValidate(m *BackstageModel, _ bsv1.Backstage) error {
	__sealights__.TraceFunc("98aebc701c5a4254db")
	m.backstageDeployment.addEnvVarsFrom([]string{BackstageContainerName()}, ConfigMapObjectKind,
		p.ConfigMap.Name, "")
	return nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapEnvs) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	__sealights__.TraceFunc("a65ec0c3ad6fc2fd07")
	p.ConfigMap.SetName(utils.GenerateRuntimeObjectName(backstage.Name, "backstage-envs"))
	setMetaInfo(p.ConfigMap, backstage, scheme)
}
