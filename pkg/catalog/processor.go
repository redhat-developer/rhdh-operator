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
	// InternalConfigMapName is the name embedded in the wrapped ConfigMap
	InternalConfigMapName = "catalog-dynamic-plugins"

	// CatalogFileName is the expected file in OCI artifacts
	CatalogFileName = "dynamic-plugins.default.yaml"

	// CatalogsReadyKey is the marker key added to ConfigMap when catalogs are ready
	CatalogsReadyKey = ".catalogs-ready"
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

	// Wrap in ConfigMap YAML and build JSON patch
	wrapped := p.wrapInConfigMap(merged)
	patchData := map[string]interface{}{
		"data": map[string]string{
			model.DynamicPluginsFile: string(wrapped),
			CatalogsReadyKey:         "true",
		},
	}

	return json.Marshal(patchData)
}

// wrapInConfigMap wraps the plugin config in a ConfigMap manifest YAML
func (p *Processor) wrapInConfigMap(content []byte) []byte {
	wrapped := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
data:
  %s: |
%s`, InternalConfigMapName, model.DynamicPluginsFile, indentYAML(string(content), 4))
	return []byte(wrapped)
}

// indentYAML indents each line of the YAML content by the specified number of spaces
func indentYAML(content string, spaces int) string {
	indent := strings.Repeat(" ", spaces)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = indent + line
		}
	}
	return strings.Join(lines, "\n")
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

// merge combines multiple catalog contents, deduplicating by package name
func (p *Processor) merge(catalogs [][]byte) ([]byte, error) {
	var allPlugins []model.DynaPlugin
	seen := make(map[string]bool)

	for _, content := range catalogs {
		var catalog model.DynaPluginsConfig
		if err := yaml.Unmarshal(content, &catalog); err != nil {
			return nil, fmt.Errorf("failed to parse catalog: %w", err)
		}
		for _, plugin := range catalog.Plugins {
			if !seen[plugin.Package] {
				allPlugins = append(allPlugins, plugin)
				seen[plugin.Package] = true
			}
		}
	}

	return yaml.Marshal(model.DynaPluginsConfig{Plugins: allPlugins})
}
