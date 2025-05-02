package model

import (
	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"
	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type ConfigMapFilesFactory struct{}

func (f ConfigMapFilesFactory) newBackstageObject() RuntimeObject {
	__sealights__.TraceFunc("ac55df224dbce50b1c")
	return &ConfigMapFiles{}
}

type ConfigMapFiles struct {
	ConfigMap *corev1.ConfigMap
}

func init() {
	__sealights__.TraceFunc("d05a018df0ab1e75a5")
	registerConfig("configmap-files.yaml", ConfigMapFilesFactory{}, false)
}

func addConfigMapFilesFromSpec(spec bsv1.BackstageSpec, model *BackstageModel) error {
	__sealights__.TraceFunc("398894dd3cb3473abd")
	if spec.Application == nil || spec.Application.ExtraFiles == nil || spec.Application.ExtraFiles.ConfigMaps == nil {
		return nil
	}

	for _, specCm := range spec.Application.ExtraFiles.ConfigMaps {

		mp, wSubpath := model.backstageDeployment.mountPath(specCm.MountPath, specCm.Key, spec.Application.ExtraFiles.MountPath)
		keys := model.ExternalConfig.ExtraFileConfigMapKeys[specCm.Name].All()
		model.backstageDeployment.mountFilesFrom([]string{BackstageContainerName()}, ConfigMapObjectKind,
			specCm.Name, mp, specCm.Key, wSubpath, keys)
	}
	return nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) Object() runtime.Object {
	__sealights__.TraceFunc("de456bb2f96e9422d2")
	return p.ConfigMap
}

func (p *ConfigMapFiles) setObject(obj runtime.Object) {
	__sealights__.TraceFunc("d987e5a468eae7d395")
	p.ConfigMap = nil
	if obj != nil {
		p.ConfigMap = obj.(*corev1.ConfigMap)
	}
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) EmptyObject() client.Object {
	__sealights__.TraceFunc("93fe508a7aabed7ab5")
	return &corev1.ConfigMap{}
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	__sealights__.TraceFunc("1cd2660972b531ec42")
	if p.ConfigMap != nil {
		model.setRuntimeObject(p)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) updateAndValidate(m *BackstageModel, _ bsv1.Backstage) error {
	__sealights__.TraceFunc("5684b6c49cddfc48f0")

	keys := append(maps.Keys(p.ConfigMap.Data), maps.Keys(p.ConfigMap.BinaryData)...)
	m.backstageDeployment.mountFilesFrom([]string{BackstageContainerName()}, ConfigMapObjectKind,
		p.ConfigMap.Name, m.backstageDeployment.defaultMountPath(), "", true, keys)

	return nil
}

func (p *ConfigMapFiles) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	__sealights__.TraceFunc("ae3a9be693f075e096")
	p.ConfigMap.SetName(utils.GenerateRuntimeObjectName(backstage.Name, "backstage-files"))
	setMetaInfo(p.ConfigMap, backstage, scheme)
}
