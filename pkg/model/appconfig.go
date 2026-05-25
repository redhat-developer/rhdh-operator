package model

import (
	"fmt"
	"path/filepath"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
	registerConfig(AppConfigKey, AppConfigFactory{}, true, mergeMultiObjectConfigs)
}

// implementation of RuntimeObject interface
func (b *AppConfig) Object() runtime.Object {
	if b.ConfigMaps != nil && len(b.ConfigMaps.Items) > 0 {
		return b.ConfigMaps
	}
	return nil
}

// implementation of RuntimeObject interface
func (b *AppConfig) addToModel(model *BackstageModel, backstage api.Backstage, config runtime.Object, scheme *runtime.Scheme) error {
	b.model = model
	if config != nil {
		b.ConfigMaps = config.(*multiobject.MultiObject)
	} else {
		// Create empty ConfigMaps - might be populated later from spec
		b.ConfigMaps = &multiobject.MultiObject{Items: []client.Object{}}
	}

	// Always add to model so updateAndValidate is called (may process spec ConfigMaps)
	model.setRuntimeObject(AppConfigKey, b)
	b.setMetaInfo(backstage, scheme)

	return nil
}

// implementation of RuntimeObject interface
func (b *AppConfig) updateAndValidate(backstage api.Backstage, scheme *runtime.Scheme) error {
	deployment := b.model.getDeployment()
	if deployment == nil {
		return fmt.Errorf("backstage deployment not found in model")
	}

	// Get plugins app-config CM if any and add to b.ConfigMaps on the first place

	// Process ConfigMaps from config files
	if b.ConfigMaps != nil {
		for _, item := range b.ConfigMaps.Items {
			cm, ok := item.(*corev1.ConfigMap)
			if !ok {
				return fmt.Errorf("payload is not ConfigMap kind: %T", item)
			}

			err := updatePodWithAppConfig(b.model.getDeployment(), cm.Name,
				b.model.getDeployment().defaultMountPath(), "", true, utils.SortedKeys(cm.Data))
			if err != nil {
				return err
			}
		}

		// Process ConfigMaps from CR spec
		if backstage.Spec.Application != nil && backstage.Spec.Application.AppConfig != nil && backstage.Spec.Application.AppConfig.ConfigMaps != nil {
			for _, specCm := range backstage.Spec.Application.AppConfig.ConfigMaps {
				mp, wSubpath := deployment.mountPath(specCm.MountPath, specCm.Key, backstage.Spec.Application.AppConfig.MountPath)
				err := updatePodWithAppConfig(deployment, specCm.Name,
					mp, specCm.Key, wSubpath, b.model.ExternalConfig.AppConfigKeys[specCm.Name])
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (b *AppConfig) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	setMultiObjectConfigMetaInfo(b.ConfigMaps, "appconfig", backstage, scheme)
}

// updatePodWithAppConfig contributes to Volumes, container.VolumeMounts and container.Args
func updatePodWithAppConfig(bsd *BackstageDeployment, cmName, mountPath, key string, withSubPath bool, cmData []string) error {

	// TODO, enable this check
	//if len(cmData) > 1 {
	//	return fmt.Errorf("multiple fields is not allowed for app-config ConfigMap %s", cmName)
	//}

	_ = bsd.mountFilesFrom(containersFilter{}, ConfigMapObjectKind,
		cmName, mountPath, key, withSubPath, cmData)
	container := bsd.container()

	for _, file := range cmData {
		if key == "" || key == file {
			container.Args = append(container.Args, []string{"--config", filepath.Join(mountPath, file)}...)
		}
	}
	return nil
}
