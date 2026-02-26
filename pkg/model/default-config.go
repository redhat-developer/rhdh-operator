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
// 2. Merge configs according to the object's FlavourMergePolicy
// 3. Fall back to base default-config if no flavours provide the file
func ReadDefaultConfig(conf ObjectConfig, key string, flavours []enabledFlavour, scheme runtime.Scheme, platformExt string) ([]client.Object, error) {

	basePath := utils.DefFile(key)

	// Step 1: Handle NoFlavour policy - use base config only
	if conf.FlavourMergePolicy == FlavourMergePolicyNoFlavour {
		if _, err := os.Stat(basePath); os.IsNotExist(err) {
			return []client.Object{}, nil
		}
		return utils.ReadYamlFiles(basePath, scheme, platformExt)
	}

	// Step 2: Collect config file paths from flavours and base
	configPaths := collectConfigPaths(key, basePath, flavours)

	// Step 3: If no configs found, return empty array (config file is optional)
	if len(configPaths) == 0 {
		return []client.Object{}, nil
	}

	// Step 4: Merge configs based on the policy
	switch conf.FlavourMergePolicy {
	case FlavourMergePolicyArrayMerge:
		return mergeDynamicPlugins(configPaths, scheme, platformExt)

	case FlavourMergePolicyMultiObject:
		configSources := collectConfigSources(key, basePath, flavours)
		return mergeMultiObjectConfigs(configSources, scheme, platformExt)
	}

	// Unreachable - all policy values handled above
	return nil, fmt.Errorf("unknown flavour merge policy: %d", conf.FlavourMergePolicy)
}

// configSource represents a config file with its source (base or flavour)
type configSource struct {
	path        string
	flavourName string // empty string for base config
}

// collectConfigPaths collects all config file paths from enabled flavours and base default-config
// Returns paths in merge order: base first, then flavours (in spec order)
// This ensures flavours override base defaults when configs are merged
func collectConfigPaths(key string, basePath string, flavours []enabledFlavour) []string {
	sources := collectConfigSources(key, basePath, flavours)
	paths := make([]string, len(sources))
	for i, src := range sources {
		paths[i] = src.path
	}
	return paths
}

// collectConfigSources collects all config file sources with flavour names
// Returns sources in merge order: base first, then flavours (in spec order)
func collectConfigSources(key string, basePath string, flavours []enabledFlavour) []configSource {
	var sources []configSource

	// Add base config if it exists
	if _, err := os.Stat(basePath); err == nil {
		sources = append(sources, configSource{
			path:        basePath,
			flavourName: "", // empty for base
		})
	}

	// Add each flavour config if it exists
	for _, flavour := range flavours {
		flavourConfigPath := filepath.Join(flavour.basePath, key)
		if _, err := os.Stat(flavourConfigPath); err == nil {
			sources = append(sources, configSource{
				path:        flavourConfigPath,
				flavourName: flavour.name,
			})
		}
	}

	return sources
}

// mergeDynamicPlugins merges dynamic-plugins.yaml files by package name
// Later entries override earlier entries with the same package name
func mergeDynamicPlugins(paths []string, scheme runtime.Scheme, platformExt string) ([]client.Object, error) {

	if len(paths) == 0 {
		return []client.Object{}, nil
	}

	var resultConfigMap *corev1.ConfigMap
	var mergedData string

	for i := 0; i < len(paths); i++ {
		path := paths[i]

		objs, err := utils.ReadYamlFiles(path, scheme, platformExt)
		if err != nil {
			return nil, fmt.Errorf("failed to read dynamic-plugins.yaml from %s: %w", path, err)
		}

		// single object expected
		configMap, ok := objs[0].(*corev1.ConfigMap)
		if !ok {
			return nil, fmt.Errorf("no ConfigMap found in %s", path)
		}

		data, ok := configMap.Data[DynamicPluginsFile]
		if !ok {
			return nil, fmt.Errorf("no %s key found in ConfigMap from %s", DynamicPluginsFile, path)
		}

		mergedData, err = MergePluginsData(mergedData, data)
		if err != nil {
			return nil, fmt.Errorf("failed to merge dynamic-plugins from %s: %w", path, err)
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
		// Read the YAML file
		objs, err := utils.ReadYamlFiles(src.path, scheme, platformExt)
		if err != nil {
			return nil, fmt.Errorf("failed to read config from %s: %w", src.path, err)
		}

		// Process each object from the file
		for _, obj := range objs {
			// If this is a flavour config, prefix the object name
			if src.flavourName != "" {
				if err := prefixObjectName(obj, src.flavourName); err != nil {
					return nil, fmt.Errorf("failed to prefix object name for flavour %s: %w", src.flavourName, err)
				}
			}

			allObjects = append(allObjects, obj)
		}
	}

	return allObjects, nil
}

// prefixObjectName adds a flavour prefix to an object's metadata.name
func prefixObjectName(obj client.Object, flavourName string) error {
	// Get the current name
	currentName := obj.GetName()
	if currentName == "" {
		return fmt.Errorf("object has no name to prefix")
	}

	// Create the new name with flavour prefix
	newName := flavourName + "-" + currentName

	// Set the new name
	obj.SetName(newName)

	return nil
}
