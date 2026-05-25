package model

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/redhat-developer/rhdh-operator/api"
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
	//EffectivePlugins      *corev1.ConfigMap
	//EffectiveAppConfig    *corev1.ConfigMap
	//EffectiveDependencies []PluginDependency
	//dynaPlugins           []DynaPlugin
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
	registerConfig(DynamicPluginsKey, DynamicPluginsFactory{}, false, mergeDynamicPlugins)
}

func DynamicPluginsDefaultName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage-dynamic-plugins")
}

// TODO to be used later
func (d *DynaPlugin) isReference() bool {
	return strings.HasPrefix(d.Package, "ref://")
}

// TODO to be used later
func (d *DynaPlugin) getName() string {
	packageURL := d.Package

	// OCI multi-plugin: oci://host/path:tag!plugin-name
	if strings.Contains(packageURL, "!") {
		parts := strings.Split(packageURL, "!")
		return parts[len(parts)-1]
	}

	// OCI single plugin: oci://host/path/plugin-name:tag or @digest
	if strings.HasPrefix(packageURL, "oci://") {
		re := regexp.MustCompile(`^oci://[^/]+/(?:.*/)?([^/:@]+)[:@]`)
		if matches := re.FindStringSubmatch(packageURL); len(matches) > 1 {
			return matches[1]
		}
	}

	// NPM: @scope/plugin-name@version
	if strings.HasPrefix(packageURL, "@") {
		// Remove scope
		parts := strings.Split(packageURL, "/")
		if len(parts) > 1 {
			packageURL = parts[1]
			// Remove @version
			versionParts := strings.Split(packageURL, "@")
			return versionParts[0]
		}
	}

	// Local: ./dynamic-plugins/dist/plugin-name
	// Reference: ref://plugin-name
	if strings.HasPrefix(packageURL, "./") || strings.HasPrefix(d.Package, "ref://") {
		parts := strings.Split(packageURL, "/")
		return parts[len(parts)-1]
	}

	return ""
}

// implementation of RuntimeObject interface
func (p *DynamicPlugins) Object() runtime.Object {
	if p.ConfigMap == nil {
		return nil
	}
	return p.ConfigMap
}

// implementation of RuntimeObject interface
func (p *DynamicPlugins) addToModel(model *BackstageModel, backstage api.Backstage, config runtime.Object, scheme *runtime.Scheme) error {
	p.model = model
	if config != nil {
		p.ConfigMap = config.(*corev1.ConfigMap)
		// Validate the ConfigMap has required data
		if p.ConfigMap.Data == nil || p.ConfigMap.Data[DynamicPluginsFile] == "" {
			return fmt.Errorf("dynamic plugin configMap expects '%s' Data key", DynamicPluginsFile)
		}
	}

	if backstage.Spec.Application != nil && backstage.Spec.Application.DynamicPluginsConfigMapName != "" {
		specPlugins := &p.model.ExternalConfig.DynamicPlugins

		// if the ConfigMap is set but does not have the data or expected key
		if specPlugins.Data == nil || specPlugins.Data[DynamicPluginsFile] == "" {
			return fmt.Errorf("dynamic plugin configMap expects '%s' Data key", DynamicPluginsFile)
		}

		//// resolve references
		// TODO
		//plugins, err := GetPluginsData(specPlugins)
		//if err != nil {
		//	return err
		//}
		//for _, plugin := range plugins {
		//	if plugin.Package
		//
		//}

		if p.ConfigMap != nil {
			// Merge user's config with default config
			//mergedData, err := p.mergeWith(specPlugins.Data[DynamicPluginsFile])
			mergedData, err := MergePluginsData(p.ConfigMap.Data[DynamicPluginsFile], specPlugins.Data[DynamicPluginsFile])
			if err != nil {
				return fmt.Errorf("failed to merge dynamic plugins config: %w", err)
			}
			p.ConfigMap.Data[DynamicPluginsFile] = mergedData
		} else {
			// No default config, point to user's config
			p.ConfigMap = specPlugins
		}
	}

	p.setMetaInfo(backstage, scheme)
	// Always add wrapper to model (unconditional)
	model.setRuntimeObject(DynamicPluginsKey, p)

	return nil
}

// implementation of RuntimeObject interface
// ConfigMap name must be the same as (deployment.yaml).spec.template.spec.volumes.name.dynamic-plugins-conf.ConfigMap.name
// TODO
// extract pluginConfigs
// merge with app-config (deep merge)
func (p *DynamicPlugins) updateAndValidate(backstage api.Backstage, scheme *runtime.Scheme) error {

	// Only proceed if there's a ConfigMap to mount or dynamic plugins config in spec
	if p.ConfigMap == nil && (backstage.Spec.Application == nil || backstage.Spec.Application.DynamicPluginsConfigMapName == "") {
		// No dynamic plugins configuration, nothing to do
		return nil
	}

	deployment := p.model.getDeployment()
	if deployment == nil {
		return fmt.Errorf("backstage deployment not found in model")
	}

	_, initContainer := p.getInitContainer()
	if initContainer == nil {
		return fmt.Errorf("failed to find initContainer named %s", dynamicPluginInitContainerName)
	}

	if err := deployment.mountFilesFrom(containersFilter{names: []string{dynamicPluginInitContainerName}}, ConfigMapObjectKind,
		p.ConfigMap.Name, initContainer.WorkingDir, DynamicPluginsFile, true, maps.Keys(p.ConfigMap.Data)); err != nil {
		return fmt.Errorf("failed to mount dynamic plugins configMap: %w", err)
	}

	return nil
}

func (p *DynamicPlugins) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	if p.ConfigMap != nil {
		p.ConfigMap.SetName(DynamicPluginsDefaultName(backstage.Name))
		setMetaInfo(p.ConfigMap, backstage, scheme)
	}
}

// Dependencies returns a list of plugin dependencies
func (p *DynamicPlugins) Dependencies() ([]PluginDependency, error) {
	//ps := p.dynaPlugins
	ps, err := GetPluginsData(p.ConfigMap)
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
func GetPluginsData(cm *corev1.ConfigMap) ([]DynaPlugin, error) {
	if cm == nil {
		return []DynaPlugin{}, nil
	}

	data := cm.Data[DynamicPluginsFile]
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

func (p *DynamicPlugins) getInitContainer() (int, *corev1.Container) {
	deployment := p.model.getDeployment()
	if deployment == nil {
		return -1, nil
	}
	i, initContainer := DynamicPluginsInitContainer(deployment.podSpec().InitContainers)
	if initContainer == nil {
		return i, nil
	}
	// override image with env var
	if os.Getenv(BackstageImageEnvVar) != "" {
		initContainer.Image = os.Getenv(BackstageImageEnvVar)
	}
	return i, initContainer
}

// returns initContainer supposed to initialize DynamicPlugins
func DynamicPluginsInitContainer(initContainers []corev1.Container) (int, *corev1.Container) {
	for i := range initContainers {
		if initContainers[i].Name == dynamicPluginInitContainerName {
			return i, &initContainers[i]
		}
	}
	return -1, nil
}

func MergePluginsData(firstData, secondData string) (string, error) {

	if firstData == "" {
		return secondData, nil
	}

	if secondData == "" {
		return firstData, nil
	}

	var firstPluginsConfig, secondPluginsConfig, mergedPluginsConfig DynaPluginsConfig

	if err := yaml.Unmarshal([]byte(firstData), &firstPluginsConfig); err != nil {
		return "", fmt.Errorf("failed to unmarshal first ConfigMap data: %w", err)
	}

	if err := yaml.Unmarshal([]byte(secondData), &secondPluginsConfig); err != nil {
		return "", fmt.Errorf("failed to unmarshal second ConfigMap data: %w", err)
	}

	// Merge Plugins by package field
	pluginMap := make(map[string]DynaPlugin)
	for _, plugin := range firstPluginsConfig.Plugins {
		pluginMap[plugin.Package] = plugin
	}
	for _, plugin := range secondPluginsConfig.Plugins {

		if existingPlugin, found := pluginMap[plugin.Package]; found {
			if plugin.PluginConfig != nil {
				existingPlugin.PluginConfig = plugin.PluginConfig
			}
			if plugin.Dependencies != nil {
				if len(plugin.Dependencies) > 0 {
					existingPlugin.Dependencies = plugin.Dependencies
				} else {
					// if dependencies is explicitly set to empty, clear it
					existingPlugin.Dependencies = []PluginDependency{}
				}
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

	if secondPluginsConfig.Includes != nil && len(secondPluginsConfig.Includes) == 0 {
		// if includes is empty explicitly, clean it
		mergedPluginsConfig.Includes = make([]string, 0)
	} else {
		// otherwise merge ensuring uniqueness
		includeSet := make(map[string]struct{})
		for _, include := range firstPluginsConfig.Includes {
			includeSet[include] = struct{}{}
		}
		for _, include := range secondPluginsConfig.Includes {
			includeSet[include] = struct{}{}
		}
		mergedPluginsConfig.Includes = make([]string, 0, len(includeSet))
		for include := range includeSet {
			mergedPluginsConfig.Includes = append(mergedPluginsConfig.Includes, include)
		}
	}

	// Marshal the merged data back to YAML
	mergedData, err := yaml.Marshal(mergedPluginsConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal merged plugins config: %w", err)
	}

	return string(mergedData), nil
}
