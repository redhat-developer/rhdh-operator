package model

import (
	"fmt"
	"os"
	"path/filepath"

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
	metadata FlavourMetadata
}

// GetEnabledFlavours determines which flavours should be enabled based on the BackstageSpec.
// It merges default flavours (enabledByDefault: true) with explicit spec.Flavours:
// - If spec.Flavours is nil: use all flavours with enabledByDefault: true
// - If spec.Flavours is provided:
//   - Explicit enabled flavours are included (even if not default)
//   - Explicit disabled flavours are excluded (even if default)
//   - Default flavours not mentioned in spec are included
//
// This should be called once per Backstage reconciliation and the result reused for all config files.
func GetEnabledFlavours(spec api.BackstageSpec) ([]enabledFlavour, error) {
	localBin := os.Getenv("LOCALBIN")
	flavoursDir := filepath.Join(localBin, "default-config", "flavours")

	// Get all default flavours
	defaults, err := getDefaultFlavours(flavoursDir)
	if err != nil {
		return nil, err
	} else if spec.Flavours == nil {
		return defaults, nil
	}

	// Merge spec.Flavours with defaults
	return mergeWithDefaults(spec.Flavours, defaults, flavoursDir)
}

// mergeWithDefaults merges spec.Flavours with default flavours
// Order: spec flavours (if enabled) first, then defaults not in spec
func mergeWithDefaults(specFlavours []api.Flavour, defaults []enabledFlavour, flavoursDir string) ([]enabledFlavour, error) {
	var result []enabledFlavour
	seen := make(map[string]bool)

	// First, process spec flavours in order
	for _, f := range specFlavours {
		seen[f.Name] = true

		// Skip disabled flavours (even if they're default)
		if !f.Enabled {
			continue
		}

		// Check if flavour directory exists
		flavourPath := filepath.Join(flavoursDir, f.Name)
		if _, err := os.Stat(flavourPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load flavour '%s': directory does not exist", f.Name)
		}

		// Load metadata for enabled flavour
		metadata, err := loadFlavourMetadata(flavourPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load flavour '%s': %w", f.Name, err)
		}

		result = append(result, enabledFlavour{
			name:     f.Name,
			basePath: flavourPath,
			metadata: *metadata,
		})
	}

	// Then, add default flavours not mentioned in spec
	for _, defaultFlavour := range defaults {
		if !seen[defaultFlavour.name] {
			result = append(result, defaultFlavour)
		}
	}

	return result, nil
}

// getDefaultFlavours returns all flavours with enabledByDefault: true
func getDefaultFlavours(flavoursDir string) ([]enabledFlavour, error) {
	// Check if flavours directory exists
	entries, err := os.ReadDir(flavoursDir)
	if err != nil {
		if os.IsNotExist(err) {
			// No flavours directory means no default flavours
			return []enabledFlavour{}, nil
		}
		return nil, fmt.Errorf("failed to read flavours directory: %w", err)
	}

	var flavours []enabledFlavour
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		flavourPath := filepath.Join(flavoursDir, entry.Name())
		metadata, err := loadFlavourMetadata(flavourPath)
		if err != nil {
			return nil, err
		}

		// Only include flavours with enabledByDefault: true
		if metadata.EnabledByDefault {
			flavours = append(flavours, enabledFlavour{
				name:     entry.Name(),
				basePath: flavourPath,
				metadata: *metadata,
			})
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
