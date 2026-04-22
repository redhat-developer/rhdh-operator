package model

import (
	"fmt"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/runtime"

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
	registerConfig(ConfigMapFilesKey, ConfigMapFilesFactory{}, true, mergeMultiObjectConfigs)
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) Object() runtime.Object {
	if p.ConfigMaps != nil && len(p.ConfigMaps.Items) > 0 {
		return p.ConfigMaps
	}
	return nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) addToModel(model *BackstageModel, backstage api.Backstage, config runtime.Object, scheme *runtime.Scheme) error {
	p.model = model
	if config != nil {
		p.ConfigMaps = config.(*multiobject.MultiObject)
	}
	//} else {
	//	// Create empty ConfigMaps - might be populated later from spec
	//	p.ConfigMaps = &multiobject.MultiObject{Items: []client.Object{}}
	//}

	// Always add to model so updateAndValidate is called (may process spec ConfigMaps)
	model.setRuntimeObject(ConfigMapFilesKey, p)
	if p.ConfigMaps != nil && len(p.ConfigMaps.Items) > 0 {
		p.setMetaInfo(backstage, scheme)
	}
	return nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) updateAndValidate(backstage api.Backstage, scheme *runtime.Scheme) error {
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

			keys := append(maps.Keys(cm.Data), maps.Keys(cm.BinaryData)...)
			mountPath, subPath, fileName := deployment.getDefConfigMountPath(cm)
			err := deployment.mountFilesFrom(containersFilter{annotation: cm.GetAnnotations()[ContainersAnnotation]}, ConfigMapObjectKind,
				cm.Name, mountPath, fileName, subPath != "", keys)

			if err != nil {
				return fmt.Errorf("failed to add files from configmap %s: %w", cm.Name, err)
			}
		}
	}

	// Process configmaps from CR spec (formerly addExternalConfig)
	if backstage.Spec.Application != nil && backstage.Spec.Application.ExtraFiles != nil && backstage.Spec.Application.ExtraFiles.ConfigMaps != nil {
		for _, specCm := range backstage.Spec.Application.ExtraFiles.ConfigMaps {

			mp, wSubpath := deployment.mountPath(specCm.MountPath, specCm.Key, backstage.Spec.Application.ExtraFiles.MountPath)
			keys := p.model.ExternalConfig.ExtraFileConfigMapKeys[specCm.Name].All()
			err := deployment.mountFilesFrom(containersFilter{names: specCm.Containers}, ConfigMapObjectKind,
				specCm.Name, mp, specCm.Key, wSubpath, keys)
			if err != nil {
				return fmt.Errorf("failed to mount files on configmap %s: %w", specCm.Name, err)
			}
		}
	}

	return nil
}

// implementation of RuntimeObject interface
func (p *ConfigMapFiles) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	setMultiObjectConfigMetaInfo(p.ConfigMaps, "files", backstage, scheme)
}
