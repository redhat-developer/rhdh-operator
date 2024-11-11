package model

import (
	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

type ConfigMapFilesFactory struct{}

func (f ConfigMapFilesFactory) newBackstageObject() RuntimeObject {
	return &ConfigMapFiles{}
}

type ConfigMapFiles struct {
	ConfigMap *corev1.ConfigMap
}

func init() {
	registerConfig("configmap-files.yaml", ConfigMapFilesFactory{}, false)
}

func addConfigMapFilesFromSpec(spec bsv1.BackstageSpec, model *BackstageModel) error {
	if spec.Application == nil || spec.Application.ExtraFiles == nil || spec.Application.ExtraFiles.ConfigMaps == nil {
		return nil
	}

	for _, specCm := range spec.Application.ExtraFiles.ConfigMaps {

		mp, wSubpath := model.backstageDeployment.mountPath(specCm.MountPath, specCm.Key, spec.Application.ExtraFiles.MountPath)
		//dataKeys := maps.Keys(model.ExternalConfig.ExtraFileConfigMaps[specCm.Name].Data)
		//binDataKeys := maps.Keys(model.ExternalConfig.ExtraFileConfigMaps[specCm.Name].Data)
		keys := append(maps.Keys(model.ExternalConfig.ExtraFileConfigMaps[specCm.Name].Data),
			maps.Keys(model.ExternalConfig.ExtraFileConfigMaps[specCm.Name].BinaryData)...)
		utils.MountFilesFrom(&model.backstageDeployment.deployment.Spec.Template.Spec, model.backstageDeployment.container(), utils.ConfigMapObjectKind,
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
	if p.ConfigMap != nil {
		model.setRuntimeObject(p)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) updateAndValidate(m *BackstageModel, _ bsv1.Backstage) error {

	keys := append(maps.Keys(p.ConfigMap.Data), maps.Keys(p.ConfigMap.BinaryData)...)
	utils.MountFilesFrom(&m.backstageDeployment.deployment.Spec.Template.Spec, m.backstageDeployment.container(), utils.ConfigMapObjectKind,
		p.ConfigMap.Name, m.backstageDeployment.defaultMountPath(), "", true, keys)

	return nil
}

func (p *ConfigMapFiles) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	p.ConfigMap.SetName(utils.GenerateRuntimeObjectName(backstage.Name, "backstage-files"))
	setMetaInfo(p.ConfigMap, backstage, scheme)
}
