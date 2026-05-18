package model

import (
	"fmt"
	"path/filepath"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
)

type AppConfigFactory struct{}

// factory method to create App Config object
func (f AppConfigFactory) newBackstageObject() RuntimeObject {
	return &AppConfig{}
}

// AppConfig represents app-config ConfigMaps from base and flavours
type AppConfig struct {
	ConfigMaps *multiobject.MultiObject
	model      *BackstageModel
}

func init() {
	registerConfig("app-config.yaml", AppConfigFactory{}, true, mergeMultiObjectConfigs)
}

func (b *AppConfig) addExternalConfig(spec api.BackstageSpec) error {

	if spec.Application == nil || spec.Application.AppConfig == nil || spec.Application.AppConfig.ConfigMaps == nil {
		return nil
	}

	for _, specCm := range spec.Application.AppConfig.ConfigMaps {
		mp, wSubpath := b.model.backstageDeployment.mountPath(specCm.MountPath, specCm.Key, spec.Application.AppConfig.MountPath)
		updatePodWithAppConfig(b.model.backstageDeployment, specCm.Name,
			mp, specCm.Key, wSubpath, b.model.ExternalConfig.AppConfigKeys[specCm.Name])
	}
	return nil
}

// implementation of RuntimeObject interface
func (b *AppConfig) Object() runtime.Object {
	return b.ConfigMaps
}

// implementation of RuntimeObject interface
func (b *AppConfig) setObject(obj runtime.Object) {
	b.ConfigMaps = nil
	if obj != nil {
		b.ConfigMaps = obj.(*multiobject.MultiObject)
	}
}

// implementation of RuntimeObject interface
func (b *AppConfig) addToModel(model *BackstageModel, _ api.Backstage) (bool, error) {
	b.model = model
	if b.ConfigMaps != nil {
		model.appConfig = b
		model.setRuntimeObject(b)
		return true, nil
	}
	return false, nil
}

// implementation of RuntimeObject interface
func (b *AppConfig) updateAndValidate(_ api.Backstage) error {
	for _, item := range b.ConfigMaps.Items {
		cm, ok := item.(*corev1.ConfigMap)
		if !ok {
			return fmt.Errorf("payload is not ConfigMap kind: %T", item)
		}

		updatePodWithAppConfig(b.model.backstageDeployment, cm.Name,
			b.model.backstageDeployment.defaultMountPath(), "", true, utils.SortedKeys(cm.Data))
	}
	return nil
}

func (b *AppConfig) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	setMultiObjectConfigMetaInfo(b.ConfigMaps, "appconfig", backstage, scheme)
}

// updatePodWithAppConfig contributes to Volumes, container.VolumeMounts and container.Args
func updatePodWithAppConfig(bsd *BackstageDeployment, cmName, mountPath, key string, withSubPath bool, cmData []string) {

	_ = bsd.mountFilesFrom(containersFilter{}, ConfigMapObjectKind,
		cmName, mountPath, key, withSubPath, cmData)
	container := bsd.container()

	for _, file := range cmData {
		if key == "" || key == file {
			container.Args = append(container.Args, []string{"--config", filepath.Join(mountPath, file)}...)
		}
	}
}
