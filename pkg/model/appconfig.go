package model

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
)

const PluginsAppConfigFile = "app-config.plugins.yaml"

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
func (b *AppConfig) GetKey() string {
	return AppConfigKey
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
	model.setRuntimeObject(b)

	return nil
}

// addPluginsAppConfig creates a ConfigMap with merged pluginConfig from all enabled plugins
// and prepends it to b.ConfigMaps.Items. Does nothing if there are no plugin configs.
func (b *AppConfig) addPluginsAppConfig(namespace string) error {
	dpObj := b.model.GetRuntimeObject(DynamicPluginsKey)
	if dpObj == nil {
		return nil
	}

	plugins := dpObj.(*DynamicPlugins).enabledPlugins
	if plugins == nil {
		return nil
	}

	mergedConfig := make(map[string]interface{})
	for _, plugin := range plugins {
		mergePluginConfigs(mergedConfig, plugin.PluginConfig)
	}
	if len(mergedConfig) == 0 {
		return nil
	}

	configYaml, err := yaml.Marshal(mergedConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal plugin configs: %w", err)
	}

	pluginsCM := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "plugins-appconfig",
			Namespace: namespace,
		},
		Data: map[string]string{PluginsAppConfigFile: string(configYaml)},
	}
	b.ConfigMaps.Items = append([]client.Object{pluginsCM}, b.ConfigMaps.Items...)
	return nil
}

// implementation of RuntimeObject interface
func (b *AppConfig) updateAndValidate(backstage api.Backstage, scheme *runtime.Scheme) error {
	deployment := b.model.getDeployment()
	if deployment == nil {
		return fmt.Errorf("backstage deployment not found in model")
	}

	// Get plugins app-config CM if any and add to b.ConfigMaps on the first place
	if err := b.addPluginsAppConfig(backstage.Namespace); err != nil {
		return err
	}

	b.setMetaInfo(backstage, scheme)

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

	// allow only single entry configMap to ensure predictable order in app-config chain
	if len(cmData) > 1 {
		return fmt.Errorf("multiple entries (%d) not allowed for app-config ConfigMap: %s; split into separate single-entry ConfigMaps", len(cmData), cmName)
	}

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

// mergePluginConfigs recursively merges src plugin config into dst. Maps are merged deeply, other values are overwritten.
func mergePluginConfigs(dst, src map[string]interface{}) {
	for k, srcVal := range src {
		srcMap := toStringKeyedMap(srcVal)
		if dstVal, exists := dst[k]; exists {
			dstMap := toStringKeyedMap(dstVal)
			if srcMap != nil && dstMap != nil {
				mergePluginConfigs(dstMap, srcMap)
				dst[k] = dstMap
				continue
			}
		}
		// Assign new value, converting maps to string-keyed
		if srcMap != nil {
			dst[k] = srcMap
		} else {
			dst[k] = srcVal
		}
	}
}

// toStringKeyedMap converts map[interface{}]interface{} (from yaml.v2) to map[string]interface{}.
// Returns the map as-is if already string-keyed, or nil if not a map.
func toStringKeyedMap(v interface{}) map[string]interface{} {
	switch m := v.(type) {
	case map[string]interface{}:
		return m
	case map[interface{}]interface{}:
		result := make(map[string]interface{})
		for key, val := range m {
			result[key.(string)] = val
		}
		return result
	}
	return nil
}
