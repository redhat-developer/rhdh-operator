package model

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/template"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReadDefaultConfig reads default configuration files, merging flavour configs as needed.
//
// It performs the following steps:
// 1. Collect config file paths from enabled flavours and base
// 2. Apply templates to all sources
// 3. Merge configs using the object's MergeFunc
func ReadDefaultConfig(conf ObjectConfig, flavours []enabledFlavour, scheme runtime.Scheme, platformExt string, templateData *template.TemplateData) ([]client.Object, error) {

	basePath := utils.DefFile(conf.Key)

	// Step 1: Collect config sources from flavours and base
	configSources := collectConfigSources(conf.Key, basePath, flavours)

	// Step 2: If no configs found, return empty array (config file is optional)
	if len(configSources) == 0 {
		return []client.Object{}, nil
	}

	// Step 3: Apply templates to all sources before merging
	for i := range configSources {
		templated, err := template.ApplyTemplate(templateData, configSources[i].content)
		if err != nil {
			return nil, fmt.Errorf("failed to apply template to %s: %w", configSources[i].path, err)
		}
		configSources[i].content = templated
	}

	// Step 4: Merge configs using the provided merge function
	return conf.MergeFunc(configSources, scheme, platformExt)
}

// configSource represents a config file with its source (base or flavour)
type configSource struct {
	path        string
	flavourName string // empty string for base config
	content     []byte // pre-read YAML content
}

// collectConfigSources collects all config file sources with flavour names and reads their content
// Returns sources in merge order: base first, then flavours (in spec order)
func collectConfigSources(key string, basePath string, flavours []enabledFlavour) []configSource {
	var sources []configSource

	// Read base config if it exists
	if _, err := os.Stat(basePath); err == nil {
		if content, err := os.ReadFile(basePath); err == nil {
			sources = append(sources, configSource{
				path:        basePath,
				flavourName: "", // empty for base
				content:     content,
			})
		}
	}

	// Read each flavour config if it exists
	for _, flavour := range flavours {
		flavourConfigPath := filepath.Join(flavour.basePath, key)
		if _, err := os.Stat(flavourConfigPath); err == nil {
			if content, err := os.ReadFile(flavourConfigPath); err == nil {
				sources = append(sources, configSource{
					path:        flavourConfigPath,
					flavourName: flavour.name,
					content:     content,
				})
			}
		}
	}

	return sources
}

// noMerge is the default merge function that just uses base config without flavour support
func noMerge(sources []configSource, scheme runtime.Scheme, platformExt string) ([]client.Object, error) {
	// Only use first source (base config), ignore flavours
	if len(sources) == 0 {
		return []client.Object{}, nil
	}

	// Read platform patch if exists
	basePath := sources[0].path
	pp, err := utils.ReadPlatformPatch(basePath, platformExt)
	if err != nil {
		return nil, fmt.Errorf("failed to read platform patch: %w", err)
	}

	return utils.ReadYamls(sources[0].content, pp, scheme)
}

// mergeDynamicPlugins merges dynamic-plugins.yaml files by package name
// Later entries override earlier entries with the same package name
func mergeDynamicPlugins(sources []configSource, scheme runtime.Scheme, _ string) ([]client.Object, error) {

	if len(sources) == 0 {
		return []client.Object{}, nil
	}

	var resultConfigMap *corev1.ConfigMap
	var mergedData string

	for _, src := range sources {
		objs, err := utils.ReadYamls(src.content, nil, scheme)
		if err != nil {
			return nil, fmt.Errorf("failed to parse dynamic-plugins.yaml from %s: %w", src.path, err)
		}

		if len(objs) == 0 {
			return nil, fmt.Errorf("no objects found in %s", src.path)
		}

		// single object expected
		configMap, ok := objs[0].(*corev1.ConfigMap)
		if !ok {
			return nil, fmt.Errorf("no ConfigMap found in %s", src.path)
		}

		data, ok := configMap.Data[DynamicPluginsFile]
		if !ok {
			return nil, fmt.Errorf("no %s key found in ConfigMap from %s", DynamicPluginsFile, src.path)
		}

		mergedData, err = MergePluginsData(mergedData, data)
		if err != nil {
			return nil, fmt.Errorf("failed to merge dynamic-plugins from %s: %w", src.path, err)
		}
		resultConfigMap = configMap
	}

	// Update the ConfigMap with merged data
	resultConfigMap.Data[DynamicPluginsFile] = mergedData

	return []client.Object{resultConfigMap}, nil
}

// mergeMultiObjectConfigs handles data files that become separate ConfigMaps/Secrets.
// Each flavour creates its own object with a unique name.
// Base config objects keep their original names.
// Flavour config objects are prefixed with the flavour name (e.g., "intelligent-assistant-app-config").
func mergeMultiObjectConfigs(sources []configSource, scheme runtime.Scheme, _ string) ([]client.Object, error) {
	if len(sources) == 0 {
		return []client.Object{}, nil
	}

	var allObjects []client.Object

	for _, src := range sources {
		objs, err := utils.ReadYamls(src.content, nil, scheme)
		if err != nil {
			return nil, fmt.Errorf("failed to parse config from %s: %w", src.path, err)
		}

		// set source annotation for information
		for _, obj := range objs {
			if src.flavourName != "" {
				utils.AddAnnotation(obj, SourceAnnotation, "flavour-"+src.flavourName)
			} else {
				utils.AddAnnotation(obj, SourceAnnotation, "default")
			}
			allObjects = append(allObjects, obj)
		}
	}

	return allObjects, nil
}

func DefaultMultiObjectName(objectType, backstageName, objectName string) string {
	return "backstage-" + objectType + "-" + backstageName + "-" + objectName
}

func setMultiObjectConfigMetaInfo(mo *multiobject.MultiObject, objectType string, backstage api.Backstage, scheme *runtime.Scheme) {
	for _, item := range mo.Items {
		utils.AddAnnotation(item, ConfiguredNameAnnotation, item.GetName())
		item.SetName(DefaultMultiObjectName(objectType, backstage.Name, item.GetName()))
		setMetaInfo(item, backstage, scheme)
	}
}
