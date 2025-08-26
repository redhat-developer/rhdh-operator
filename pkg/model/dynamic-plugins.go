package model

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"

	"golang.org/x/exp/maps"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha4"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

//it relies on implementation where dynamic-plugin initContainer
//uses specified ConfigMap for producing app-config with dynamic-plugins
//For this implementation:
//- backstage contaier and dynamic-plugin initContainer must share a volume
//  where initContainer writes and backstage container reads produced app-config
//- app-config path should be set as a --config parameter of backstage container
//in the deployment manifest

//it creates a volume with dynamic-plugins ConfigMap (there should be a key named "dynamic-plugins.yaml")
//and mount it to the dynamic-plugin initContainer's WorkingDir (what if not specified?)

const dynamicPluginInitContainerName = "install-dynamic-plugins"
const DynamicPluginsFile = "dynamic-plugins.yaml"

type DynamicPluginsFactory struct{}

func (f DynamicPluginsFactory) newBackstageObject() RuntimeObject {
	return &DynamicPlugins{}
}

type DynamicPlugins struct {
	ConfigMap *corev1.ConfigMap
	model     *BackstageModel
}

type DynaPluginsConfig struct {
	// we do not really support Includes here, that's what is processed by the installation script
	// in the dynamic-plugins container. Keeping it here for the sake of completeness
	Includes []string     `yaml:"includes,omitempty"`
	Plugins  []DynaPlugin `yaml:"plugins,omitempty"`
}

type DynaPlugin struct {
	Package      string                 `yaml:"package,omitempty"`
	Integrity    string                 `yaml:"integrity,omitempty"`
	Disabled     bool                   `yaml:"disabled"`
	PluginConfig map[string]interface{} `yaml:"pluginConfig,omitempty"`
	Dependencies []PluginDependency     `yaml:"dependencies,omitempty"`
}

type PluginDependency struct {
	Ref string `yaml:"ref"`
}

func init() {
	registerConfig("dynamic-plugins.yaml", DynamicPluginsFactory{}, false)
}

func DynamicPluginsDefaultName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-dynamic-plugins")
}

// implementation of RuntimeObject interface
func (p *DynamicPlugins) Object() runtime.Object {
	return p.ConfigMap
}

func (p *DynamicPlugins) setObject(obj runtime.Object) {
	p.ConfigMap = nil
	if obj != nil {
		p.ConfigMap = obj.(*corev1.ConfigMap)
	}
}

// implementation of RuntimeObject interface
func (p *DynamicPlugins) EmptyObject() client.Object {
	return &corev1.ConfigMap{}
}

// implementation of RuntimeObject interface
func (p *DynamicPlugins) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	p.model = model
	if p.ConfigMap == nil {
		return false, nil
	}

	// if the ConfigMap is set but does not have the data or expected key
	if p.ConfigMap.Data == nil || p.ConfigMap.Data[DynamicPluginsFile] == "" {
		return false, fmt.Errorf("dynamic plugin configMap expects '%s' Data key", DynamicPluginsFile)
	}

	model.setRuntimeObject(p)
	model.DynamicPlugins = *p
	return true, nil
}

// implementation of RuntimeObject interface
// ConfigMap name must be the same as (deployment.yaml).spec.template.spec.volumes.name.dynamic-plugins-conf.ConfigMap.name
func (p *DynamicPlugins) updateAndValidate(backstage bsv1.Backstage) error {

	_, initContainer := p.getInitContainer()
	if initContainer == nil {
		return fmt.Errorf("failed to find initContainer named %s", dynamicPluginInitContainerName)
	}
	if backstage.Spec.Application == nil || backstage.Spec.Application.DynamicPluginsConfigMapName == "" {
		p.model.backstageDeployment.mountFilesFrom(containersFilter{names: []string{dynamicPluginInitContainerName}}, ConfigMapObjectKind,
			p.ConfigMap.Name, initContainer.WorkingDir, DynamicPluginsFile, true, maps.Keys(p.ConfigMap.Data))
	}

	return nil
}

// implementation of RuntimeObject interface
func (p *DynamicPlugins) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	p.ConfigMap.SetName(DynamicPluginsDefaultName(backstage.Name))
	setMetaInfo(p.ConfigMap, backstage, scheme)
}

func (p *DynamicPlugins) addExternalConfig(spec bsv1.BackstageSpec) error {
	if spec.Application != nil && spec.Application.DynamicPluginsConfigMapName != "" {

		_, initContainer := p.getInitContainer()
		if initContainer == nil {
			return fmt.Errorf("failed to find initContainer named %s", dynamicPluginInitContainerName)
		}

		dp := &p.model.ExternalConfig.DynamicPlugins

		// if the ConfigMap is set but does not have the data or expected key
		if dp.Data == nil || dp.Data[DynamicPluginsFile] == "" {
			return fmt.Errorf("dynamic plugin configMap expects '%s' Data key", DynamicPluginsFile)
		}
		if p.ConfigMap != nil {
			mergedData, err := p.mergeWith(dp.Data[DynamicPluginsFile])
			if err != nil {
				return fmt.Errorf("failed to merge dynamic plugins config: %w", err)
			}
			p.ConfigMap.Data[DynamicPluginsFile] = mergedData
			p.model.backstageDeployment.mountFilesFrom(containersFilter{names: []string{dynamicPluginInitContainerName}}, ConfigMapObjectKind,
				p.ConfigMap.Name, initContainer.WorkingDir, DynamicPluginsFile, true, maps.Keys(p.ConfigMap.Data))
		} else {
			p.model.backstageDeployment.mountFilesFrom(containersFilter{names: []string{dynamicPluginInitContainerName}}, ConfigMapObjectKind,
				dp.Name, initContainer.WorkingDir, DynamicPluginsFile, true, maps.Keys(dp.Data))
		}

	}
	return nil
}

// Dependencies returns a list of plugin dependencies
func (p *DynamicPlugins) Dependencies() ([]PluginDependency, error) {
	ps, err := p.GetPlugins()
	if err != nil {
		return nil, err
	}

	result := make([]PluginDependency, 0)

	for _, pp := range ps {
		if pp.Disabled {
			continue
		}

		result = append(result, pp.Dependencies...)
	}

	return result, nil
}

// returns a list of plugins from the configMap
func (p *DynamicPlugins) GetPlugins() ([]DynaPlugin, error) {
	if p.ConfigMap == nil {
		return []DynaPlugin{}, nil
	}

	data := p.ConfigMap.Data[DynamicPluginsFile]
	if data == "" {
		return []DynaPlugin{}, nil
	}

	var pluginsConfig DynaPluginsConfig
	err := yaml.Unmarshal([]byte(data), &pluginsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal dynamic plugins data: %w", err)
	}

	return pluginsConfig.Plugins, nil
}

func (p *DynamicPlugins) mergeWith(specData string) (string, error) {

	if p.ConfigMap == nil {
		return "", fmt.Errorf("dynamic plugins ConfigMap is not set")
	}
	modelData := p.ConfigMap.Data[DynamicPluginsFile]
	var modelPluginsConfig, specPluginsConfig, mergedPluginsConfig DynaPluginsConfig
	if modelData != "" {
		if err := yaml.Unmarshal([]byte(modelData), &modelPluginsConfig); err != nil {
			return "", fmt.Errorf("failed to unmarshal model ConfigMap data: %w", err)
		}
	}
	if specData != "" {
		if err := yaml.Unmarshal([]byte(specData), &specPluginsConfig); err != nil {
			return "", fmt.Errorf("failed to unmarshal spec ConfigMap data: %w", err)
		}
	}

	// Merge Plugins by package field
	pluginMap := make(map[string]DynaPlugin)
	for _, plugin := range modelPluginsConfig.Plugins {
		pluginMap[plugin.Package] = plugin
	}
	for _, plugin := range specPluginsConfig.Plugins {

		if existingPlugin, found := pluginMap[plugin.Package]; found {
			if plugin.PluginConfig != nil {
				existingPlugin.PluginConfig = plugin.PluginConfig
			}
			if len(plugin.Dependencies) > 0 {
				existingPlugin.Dependencies = plugin.Dependencies
			}
			if plugin.Integrity != "" {
				existingPlugin.Integrity = plugin.Integrity
			}
			existingPlugin.Disabled = plugin.Disabled
			pluginMap[plugin.Package] = existingPlugin
		} else {
			pluginMap[plugin.Package] = plugin
		}
	}
	mergedPluginsConfig.Plugins = make([]DynaPlugin, 0, len(pluginMap))
	for _, plugin := range pluginMap {
		mergedPluginsConfig.Plugins = append(mergedPluginsConfig.Plugins, plugin)
	}

	// Merge Includes (ensure uniqueness)
	includeSet := make(map[string]struct{})
	for _, include := range modelPluginsConfig.Includes {
		includeSet[include] = struct{}{}
	}
	for _, include := range specPluginsConfig.Includes {
		includeSet[include] = struct{}{}
	}
	mergedPluginsConfig.Includes = make([]string, 0, len(includeSet))
	for include := range includeSet {
		mergedPluginsConfig.Includes = append(mergedPluginsConfig.Includes, include)
	}

	// Marshal the merged data back to YAML
	mergedData, err := yaml.Marshal(mergedPluginsConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal merged plugins config: %w", err)
	}

	return string(mergedData), nil
}
func (p *DynamicPlugins) getInitContainer() (int, *corev1.Container) {
	i, initContainer := DynamicPluginsInitContainer(p.model.backstageDeployment.deployment.Spec.Template.Spec.InitContainers)

	// override image with env var
	if os.Getenv(BackstageImageEnvVar) != "" {
		initContainer.Image = os.Getenv(BackstageImageEnvVar)
	}
	return i, initContainer
}

// returns initContainer supposed to initialize DynamicPlugins
// TODO consider to use a label to identify instead
func DynamicPluginsInitContainer(initContainers []corev1.Container) (int, *corev1.Container) {
	for i, ic := range initContainers {
		if ic.Name == dynamicPluginInitContainerName {
			return i, &ic
		}
	}
	return -1, nil
}
