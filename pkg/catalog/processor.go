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

// Process fetches all catalogs, merges them, and returns JSON patch bytes ready to apply
func (p *Processor) Process(ctx context.Context, catalogs []CatalogInput) ([]byte, error) {
	var allContent [][]byte

	for _, cat := range catalogs {
		content, err := p.fetch(ctx, cat)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch catalog %s: %w", cat.Ref, err)
		}
		allContent = append(allContent, content)
	}

	merged, err := p.merge(allContent)
	if err != nil {
		return nil, fmt.Errorf("failed to merge catalogs: %w", err)
	}

	// Build the inner ConfigMap that will be stored as the value in default-config.
	// The default-config ConfigMap stores Kubernetes manifests as file values,
	// so each value must be a complete YAML document (apiVersion, kind, etc.)
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

// merge combines multiple catalog contents, validating no duplicate plugin names exist.
// Plugins are identified by name (last path segment of the package URL, without tag/digest).
// Returns an error if duplicate plugin names are found across catalogs.
func (p *Processor) merge(catalogs [][]byte) ([]byte, error) {
	var allPlugins []model.DynaPlugin
	seen := make(map[string]string) // plugin name -> package URL (for error reporting)

	for _, content := range catalogs {
		var catalog model.DynaPluginsConfig
		if err := yaml.Unmarshal(content, &catalog); err != nil {
			return nil, fmt.Errorf("failed to parse catalog: %w", err)
		}
		for _, plugin := range catalog.Plugins {
			name := plugin.Name()
			if name == "" {
				return nil, fmt.Errorf("invalid plugin package %q: cannot extract plugin name", plugin.Package)
			}
			if existingPkg, exists := seen[name]; exists {
				return nil, fmt.Errorf("duplicate plugin name %q: found in both %q and %q", name, existingPkg, plugin.Package)
			}
			allPlugins = append(allPlugins, plugin)
			seen[name] = plugin.Package
		}
	}

	return yaml.Marshal(model.DynaPluginsConfig{Plugins: allPlugins})
}
