package model

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"

	"golang.org/x/exp/maps"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
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
}

type DynaPluginsConfig struct {
	// we do not really support Includes here, that's what is processed by the installation script
	// in the dynamic-plugins container. Keeping it here for the sake of completeness
	Includes []string     `yaml:"includes"`
	Plugins  []DynaPlugin `yaml:"plugins"`
}

type DynaPlugin struct {
	Package      string                 `yaml:"package"`
	Integrity    string                 `yaml:"integrity"`
	Disabled     bool                   `yaml:"disabled"`
	PluginConfig map[string]interface{} `yaml:"pluginConfig"`
	Dependencies []PluginDependency     `yaml:"dependencies"`
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

func addDynamicPluginsFromSpec(spec bsv1.BackstageSpec, model *BackstageModel) error {

	if spec.Application == nil || spec.Application.DynamicPluginsConfigMapName == "" {
		return nil
	}

	_, ic := DynamicPluginsInitContainer(model.backstageDeployment.deployment.Spec.Template.Spec.InitContainers)
	if ic == nil {
		return fmt.Errorf("validation failed, dynamic plugin name configured but no InitContainer %s defined", dynamicPluginInitContainerName)
	}

	dp := model.ExternalConfig.DynamicPlugins

	if dp.Data == nil || dp.Data[DynamicPluginsFile] == "" {
		return fmt.Errorf("dynamic plugin configMap expects '%s' Data key", DynamicPluginsFile)
	}

	if dp.Data[DynamicPluginsFile] != "" {
		model.backstageDeployment.mountFilesFrom([]string{dynamicPluginInitContainerName}, ConfigMapObjectKind,
			dp.Name, ic.WorkingDir, DynamicPluginsFile, true, maps.Keys(dp.Data))
	}

	// if default dynamic-plugins set, the value from model.DynamicPlugins will differ from the one in model.RuntimeObjects
	model.DynamicPlugins.ConfigMap = &dp

	return nil
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
func (p *DynamicPlugins) addToModel(model *BackstageModel, backstage bsv1.Backstage) (bool, error) {

	if p.ConfigMap == nil {
		if backstage.Spec.Application != nil && backstage.Spec.Application.DynamicPluginsConfigMapName != "" {
			p.ConfigMap = &corev1.ConfigMap{}
		} else {
			return false, nil
		}
	}
	model.setRuntimeObject(p)
	model.DynamicPlugins = *p
	return true, nil
}

// implementation of RuntimeObject interface
// ConfigMap name must be the same as (deployment.yaml).spec.template.spec.volumes.name.dynamic-plugins-conf.ConfigMap.name
func (p *DynamicPlugins) updateAndValidate(model *BackstageModel, backstage bsv1.Backstage) error {

	if backstage.Spec.Application != nil && backstage.Spec.Application.DynamicPluginsConfigMapName != "" {
		return nil
	}

	_, initContainer := DynamicPluginsInitContainer(model.backstageDeployment.deployment.Spec.Template.Spec.InitContainers)
	if initContainer == nil {
		return fmt.Errorf("failed to find initContainer named %s", dynamicPluginInitContainerName)
	}
	// override image with env var
	// [GA] Do we need this feature?
	if os.Getenv(BackstageImageEnvVar) != "" {
		// TODO workaround for the (janus-idp, rhdh) case where we have
		// exactly the same image for initContainer and want it to be overriden
		// the same way as Backstage's one
		initContainer.Image = os.Getenv(BackstageImageEnvVar)
	}

	model.backstageDeployment.mountFilesFrom([]string{dynamicPluginInitContainerName}, ConfigMapObjectKind,
		p.ConfigMap.Name, initContainer.WorkingDir, DynamicPluginsFile, true, maps.Keys(p.ConfigMap.Data))

	return nil
}

// implementation of RuntimeObject interface
func (p *DynamicPlugins) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	p.ConfigMap.SetName(DynamicPluginsDefaultName(backstage.Name))
	setMetaInfo(p.ConfigMap, backstage, scheme)
}

// Dependencies returns a list of plugin dependencies
func (p *DynamicPlugins) Dependencies() ([]PluginDependency, error) {
	ps, err := p.pluginsFromConfigMap()
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
func (p *DynamicPlugins) pluginsFromConfigMap() ([]DynaPlugin, error) {
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
