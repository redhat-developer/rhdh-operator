package model

import (
	"fmt"

	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"

	corev1 "k8s.io/api/core/v1"
)

type ConfigMapFilesFactory struct{}

func (f ConfigMapFilesFactory) newBackstageObject() RuntimeObject {
	return &ConfigMapFiles{}
}

type ConfigMapFiles struct {
	ConfigMaps *multiobject.MultiObject
	model      *BackstageModel
}

func init() {
	registerConfig("configmap-files.yaml", ConfigMapFilesFactory{}, true, mergeMultiObjectConfigs)
}

func ConfigMapFilesDefaultName(backstageName, src string) string {
	if src == "" {
		return "backstage-files-" + backstageName
	}
	return "backstage-files-" + src + "-" + backstageName
}

func (p *ConfigMapFiles) addExternalConfig(spec api.BackstageSpec) error {
	if spec.Application == nil || spec.Application.ExtraFiles == nil || spec.Application.ExtraFiles.ConfigMaps == nil {
		return nil
	}

	for _, specCm := range spec.Application.ExtraFiles.ConfigMaps {

		mp, wSubpath := p.model.backstageDeployment.mountPath(specCm.MountPath, specCm.Key, spec.Application.ExtraFiles.MountPath)
		keys := p.model.ExternalConfig.ExtraFileConfigMapKeys[specCm.Name].All()
		err := p.model.backstageDeployment.mountFilesFrom(containersFilter{names: specCm.Containers}, ConfigMapObjectKind,
			specCm.Name, mp, specCm.Key, wSubpath, keys)
		if err != nil {
			return fmt.Errorf("failed to mount files on configmap %s: %w", specCm.Name, err)
		}
	}
	return nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) Object() runtime.Object {
	return p.ConfigMaps
}

func (p *ConfigMapFiles) setObject(obj runtime.Object) {
	p.ConfigMaps = nil
	if obj != nil {
		p.ConfigMaps = obj.(*multiobject.MultiObject)
	}
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) addToModel(model *BackstageModel, _ api.Backstage) (bool, error) {
	p.model = model
	if p.ConfigMaps != nil {
		model.setRuntimeObject(p)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) updateAndValidate(_ api.Backstage) error {
	for _, item := range p.ConfigMaps.Items {
		cm, ok := item.(*corev1.ConfigMap)
		if !ok {
			return fmt.Errorf("payload is not ConfigMap kind: %T", item)
		}

		keys := append(maps.Keys(cm.Data), maps.Keys(cm.BinaryData)...)
		err := p.model.backstageDeployment.mountFilesFrom(containersFilter{annotation: cm.GetAnnotations()[ContainersAnnotation]}, ConfigMapObjectKind,
			cm.Name, p.model.backstageDeployment.defaultMountPath(), "", true, keys)

		if err != nil {
			return fmt.Errorf("failed to add files from configmap %s: %w", cm.Name, err)
		}
	}
	return nil
}

func (p *ConfigMapFiles) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	for _, item := range p.ConfigMaps.Items {
		cm := item.(*corev1.ConfigMap)
		cm.Name = ConfigMapFilesDefaultName(backstage.Name, cm.Annotations[SourceAnnotation])
		setMetaInfo(cm, backstage, scheme)
	}
}
