package model

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"sigs.k8s.io/yaml"

	"github.com/redhat-developer/rhdh-operator/api"
)

// FlavourMetadata represents the metadata.yaml file in a flavour directory
type FlavourMetadata struct {
	// EnabledByDefault controls whether this flavour is enabled when spec.flavours is not specified
	EnabledByDefault bool `yaml:"enabledByDefault"`
}

// enabledFlavour represents a flavour that is enabled for this Backstage instance
type enabledFlavour struct {
	name     string
	basePath string
}

// GetEnabledFlavours determines which flavours should be enabled based on the BackstageSpec.
// Algorithm:
// 1. Load all available flavours with their enabledByDefault status from metadata.yaml
// 2. Override enabled status with values from spec.Flavours (if provided)
// 3. Return only the enabled flavours
//
// This should be called once per Backstage reconciliation and the result reused for all config files.
func GetEnabledFlavours(spec api.BackstageSpec) ([]enabledFlavour, error) {

	// if explicit empty array specified - disable all
	if spec.Flavours != nil && len(*spec.Flavours) == 0 {
		return []enabledFlavour{}, nil
	}

	localBin := os.Getenv("LOCALBIN")
	flavoursDir := filepath.Join(localBin, "default-config", "flavours")

	// Step 1: Load all flavours with their default enabled status
	allFlavours, err := loadAllFlavours(flavoursDir)
	if err != nil {
		return nil, err
	}

	// Step 2: Override enabled status from spec and preserve the order in which
	// explicitly configured flavours were declared.
	var explicitOrder []string
	explicitNames := make(map[string]struct{})
	if spec.Flavours != nil {
		flavours := *spec.Flavours

		for _, f := range flavours {
			flavour, exists := allFlavours[f.Name]
			if !exists {
				return nil, fmt.Errorf("flavour '%s' not found in %s", f.Name, flavoursDir)
			}
			flavour.enabled = f.Enabled
			allFlavours[f.Name] = flavour
			if _, seen := explicitNames[f.Name]; !seen {
				explicitOrder = append(explicitOrder, f.Name)
				explicitNames[f.Name] = struct{}{}
			}
		}
	}

	// Step 3: Apply unmentioned defaults first in stable order, then explicitly
	// configured flavours in CR declaration order. Flavour order affects merge
	// precedence and mount order, so it must not depend on map iteration or names.
	var defaultNames []string
	for name, flavour := range allFlavours {
		if _, explicit := explicitNames[name]; !explicit && flavour.enabled {
			defaultNames = append(defaultNames, name)
		}
	}
	sort.Strings(defaultNames)

	var result []enabledFlavour
	appendFlavour := func(name string) {
		result = append(result, enabledFlavour{
			name:     name,
			basePath: allFlavours[name].basePath,
		})
	}
	for _, name := range defaultNames {
		appendFlavour(name)
	}
	for _, name := range explicitOrder {
		if allFlavours[name].enabled {
			appendFlavour(name)
		}
	}

	return result, nil
}

// flavourInfo holds information about a discovered flavour
type flavourInfo struct {
	basePath string
	enabled  bool
}

// loadAllFlavours loads all available flavours from the flavours directory
// Returns a map of flavourName -> flavourInfo (with basePath and default enabled status)
func loadAllFlavours(flavoursDir string) (map[string]flavourInfo, error) {
	entries, err := os.ReadDir(flavoursDir)
	if err != nil {
		if os.IsNotExist(err) {
			// No flavours directory means no flavours available
			return make(map[string]flavourInfo), nil
		}
		return nil, fmt.Errorf("failed to read flavours directory: %w", err)
	}

	flavours := make(map[string]flavourInfo)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		flavourName := entry.Name()
		flavourPath := filepath.Join(flavoursDir, flavourName)

		// Load metadata to get enabledByDefault
		metadata, err := loadFlavourMetadata(flavourPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load flavour '%s': %w", flavourName, err)
		}

		flavours[flavourName] = flavourInfo{
			basePath: flavourPath,
			enabled:  metadata.EnabledByDefault,
		}
	}

	return flavours, nil
}

// loadFlavourMetadata loads metadata.yaml from a flavour directory
func loadFlavourMetadata(flavourPath string) (*FlavourMetadata, error) {
	metadataPath := filepath.Join(flavourPath, "metadata.yaml")

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No metadata file means enabledByDefault: false
			return &FlavourMetadata{EnabledByDefault: false}, nil
		}
		return nil, fmt.Errorf("failed to read metadata.yaml: %w", err)
	}

	var metadata FlavourMetadata
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata.yaml: %w", err)
	}

	return &metadata, nil
}
