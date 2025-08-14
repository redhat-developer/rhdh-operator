package model

import (
	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha4"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type ConfigMapFilesFactory struct{}

func (f ConfigMapFilesFactory) newBackstageObject() RuntimeObject {
	return &ConfigMapFiles{}
}

type ConfigMapFiles struct {
	ConfigMap *corev1.ConfigMap
	model     *BackstageModel
}

func init() {
	registerConfig("configmap-files.yaml", ConfigMapFilesFactory{}, false)
}

func (p *ConfigMapFiles) addExternalConfig(spec bsv1.BackstageSpec) error {
	if spec.Application == nil || spec.Application.ExtraFiles == nil || spec.Application.ExtraFiles.ConfigMaps == nil {
		return nil
	}

	for _, specCm := range spec.Application.ExtraFiles.ConfigMaps {

		mp, wSubpath := p.model.backstageDeployment.mountPath(specCm.MountPath, specCm.Key, spec.Application.ExtraFiles.MountPath)
		keys := p.model.ExternalConfig.ExtraFileConfigMapKeys[specCm.Name].All()
		p.model.backstageDeployment.mountFilesFrom([]string{BackstageContainerName()}, ConfigMapObjectKind,
			specCm.Name, mp, specCm.Key, wSubpath, keys)
	}
	return nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) Object() runtime.Object {
	return p.ConfigMap
}

func (p *ConfigMapFiles) setObject(obj runtime.Object) {
	p.ConfigMap = nil
	if obj != nil {
		p.ConfigMap = obj.(*corev1.ConfigMap)
	}
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) EmptyObject() client.Object {
	return &corev1.ConfigMap{}
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	p.model = model
	if p.ConfigMap != nil {
		model.setRuntimeObject(p)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) updateAndValidate(_ bsv1.Backstage) error {

	keys := append(maps.Keys(p.ConfigMap.Data), maps.Keys(p.ConfigMap.BinaryData)...)
	p.model.backstageDeployment.mountFilesFrom([]string{BackstageContainerName()}, ConfigMapObjectKind,
		p.ConfigMap.Name, p.model.backstageDeployment.defaultMountPath(), "", true, keys)

	return nil
}

func (p *ConfigMapFiles) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	p.ConfigMap.SetName(utils.GenerateRuntimeObjectName(backstage.Name, "backstage-files"))
	setMetaInfo(p.ConfigMap, backstage, scheme)
}
