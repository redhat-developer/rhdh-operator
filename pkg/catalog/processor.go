package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/redhat-developer/rhdh-operator/pkg/fetcher"
	"github.com/redhat-developer/rhdh-operator/pkg/model"
)

const (
	// CatalogFileName is the expected file in OCI artifacts
	CatalogFileName = "dynamic-plugins.default.yaml"
)

// Processor handles fetching, processing, and merging plugin catalogs
type Processor struct{}

// NewProcessor creates a new catalog processor
func NewProcessor() *Processor {
	return &Processor{}
}

// CatalogInput contains everything needed to fetch and process a catalog
type CatalogInput struct {
	Ref           string
	DockerConfig  []byte
	CACert        []byte
	SkipTLSVerify bool
}

// PluginMap maps plugin names to plugins for duplicate detection
type PluginMap map[string]model.DynaPlugin

// AddCatalog fetches a single catalog and adds its plugins to the map, checking for duplicates
func (p *Processor) AddCatalog(ctx context.Context, catalog CatalogInput, plugins PluginMap) error {
	content, err := p.fetch(ctx, catalog)
	if err != nil {
		return fmt.Errorf("failed to fetch catalog %s: %w", catalog.Ref, err)
	}

	var config model.DynaPluginsConfig
	if err := yaml.Unmarshal(content, &config); err != nil {
		return fmt.Errorf("failed to parse catalog %s: %w", catalog.Ref, err)
	}

	for _, plugin := range config.Plugins {
		name := plugin.Name()
		if name == "" {
			return fmt.Errorf("invalid plugin package %q: cannot extract plugin name", plugin.Package)
		}
		if existing, exists := plugins[name]; exists {
			return fmt.Errorf("duplicate plugin name %q: found in both %q and %q", name, existing.Package, plugin.Package)
		}
		plugins[name] = plugin
	}

	return nil
}

// BuildPatch creates a JSON patch for the default-config ConfigMap from accumulated plugins
func (p *Processor) BuildPatch(plugins PluginMap) ([]byte, error) {
	// Convert map to slice
	list := make([]model.DynaPlugin, 0, len(plugins))
	for _, plugin := range plugins {
		list = append(list, plugin)
	}

	merged, err := yaml.Marshal(model.DynaPluginsConfig{Plugins: list})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal plugins: %w", err)
	}

	// Build the inner ConfigMap that will be stored as the value in default-config.
	innerConfigMap := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name": "default-dynamic-plugins",
			"annotations": map[string]string{
				"rhdh.redhat.com/managed-by": "DevHubPluginCatalog",
			},
		},
		"data": map[string]string{
			model.DynamicPluginsFile: string(merged),
		},
	}

	innerYAML, err := yaml.Marshal(innerConfigMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal inner ConfigMap: %w", err)
	}

	// Build JSON patch for the default-config ConfigMap
	patchData := map[string]interface{}{
		"data": map[string]string{
			model.DynamicPluginsFile: string(innerYAML),
		},
	}

	return json.Marshal(patchData)
}

// fetch downloads a single catalog
func (p *Processor) fetch(ctx context.Context, cat CatalogInput) ([]byte, error) {
	var opts []fetcher.OCIOption
	if cat.SkipTLSVerify {
		opts = append(opts, fetcher.WithInsecure())
	}
	if cat.DockerConfig != nil {
		opts = append(opts, fetcher.WithDockerConfig(cat.DockerConfig))
	}
	if cat.CACert != nil {
		opts = append(opts, fetcher.WithCACert(cat.CACert))
	}

	ociFetcher := fetcher.NewOCIFetcher(opts...)

	tempDir, err := os.MkdirTemp("", "dhpc-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ref := strings.TrimPrefix(cat.Ref, "oci://")
	if err := ociFetcher.Fetch(ctx, ref, tempDir); err != nil {
		return nil, fmt.Errorf("failed to fetch OCI artifact: %w", err)
	}

	content, err := os.ReadFile(filepath.Join(tempDir, CatalogFileName))
	if err != nil {
		return nil, fmt.Errorf("failed to read catalog file: %w", err)
	}

	return content, nil
}
