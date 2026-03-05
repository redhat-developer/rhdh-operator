package model

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReadDefaultConfig reads default configuration files, merging flavour configs as needed.
// This is the main entry point that replaces utils.ReadYamlFiles(utils.DefFile(key), scheme, platformExt).
//
// It performs the following steps:
// 1. Collect config file paths from enabled flavours and base
// 2. Merge configs using the object's MergeFunc if provided
// 3. Fall back to base default-config if no merge function or no flavours
func ReadDefaultConfig(conf ObjectConfig, flavours []enabledFlavour, scheme runtime.Scheme, platformExt string) ([]client.Object, error) {

	basePath := utils.DefFile(conf.Key)

	// Step 1: Handle no merge function - use base config only (no flavour support)
	if conf.MergeFunc == nil {
		if _, err := os.Stat(basePath); os.IsNotExist(err) {
			return []client.Object{}, nil
		}
		return utils.ReadYamlFiles(basePath, scheme, platformExt)
	}

	// Step 2: Collect config sources from flavours and base
	configSources := collectConfigSources(conf.Key, basePath, flavours)

	// Step 3: If no configs found, return empty array (config file is optional)
	if len(configSources) == 0 {
		return []client.Object{}, nil
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

// mergeDynamicPlugins merges dynamic-plugins.yaml files by package name
// Later entries override earlier entries with the same package name
func mergeDynamicPlugins(sources []configSource, scheme runtime.Scheme, platformExt string) ([]client.Object, error) {

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
// Flavour config objects are prefixed with the flavour name (e.g., "lightspeed-app-config").
func mergeMultiObjectConfigs(sources []configSource, scheme runtime.Scheme, platformExt string) ([]client.Object, error) {
	if len(sources) == 0 {
		return []client.Object{}, nil
	}

	var allObjects []client.Object

	for _, src := range sources {
		objs, err := utils.ReadYamls(src.content, nil, scheme)
		if err != nil {
			return nil, fmt.Errorf("failed to parse config from %s: %w", src.path, err)
		}

		for _, obj := range objs {
			// If this is a flavour config, add a source annotation
			if src.flavourName != "" {
				utils.AddAnnotation(obj, SourceAnnotation, fmt.Sprintf("flavour-%s", src.flavourName))
			}
			allObjects = append(allObjects, obj)
		}
	}

	return allObjects, nil
}
