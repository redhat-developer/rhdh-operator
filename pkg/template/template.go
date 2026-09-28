package template

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// TemplateData provides values for Go template substitution in config files.
// Use {{.Rhdh.Name}}, {{.Rhdh.Namespace}}, {{.Rhdh.Runtime.Platform}}, etc. in YAML files.
type TemplateData struct {
	Rhdh RhdhData
}

// RhdhData contains RHDH runtime information available for templating in config files.
type RhdhData struct {
	Name      string
	Namespace string
	Runtime   RuntimeInfo
}

// RuntimeInfo contains platform and cluster runtime information.
type RuntimeInfo struct {
	Platform      string // "ocp" or "k8s" (platform extension)
	IngressDomain string // OpenShift ingress domain (empty on k8s or if unavailable)
}

// NewTemplateData constructs template data for YAML file processing.
// Accepts structured objects for extensibility - new template fields can be added
// by reading additional data from these objects without changing the signature.
func NewTemplateData(backstage BackstageCR, platform Platform, externalConfig ExternalConfig) *TemplateData {
	return &TemplateData{
		Rhdh: RhdhData{
			Name:      backstage.GetName(),
			Namespace: backstage.GetNamespace(),
			Runtime: RuntimeInfo{
				Platform:      platform.GetExtension(),
				IngressDomain: externalConfig.GetIngressDomain(),
			},
		},
	}
}

// Interfaces for template data sources - allows flexibility in implementation
type BackstageCR interface {
	GetName() string
	GetNamespace() string
}

type Platform interface {
	GetExtension() string
}

type ExternalConfig interface {
	GetIngressDomain() string
}

// ApplyTemplate applies Go template substitution to content if templateData is set
// and the content contains our template variables ({{.Rhdh.}}).
// Returns content unchanged if no template data has been set or no template variables found.
func ApplyTemplate(data *TemplateData, content []byte) ([]byte, error) {
	if data == nil {
		return content, nil
	}
	// Only parse as template if our specific variables are present
	// This avoids parsing errors from other {{...}} patterns in config files
	// Check for Go template patterns that reference .Rhdh. (variable references or in conditionals)
	contentStr := string(content)
	if !strings.Contains(contentStr, "{{.Rhdh.") &&
		!strings.Contains(contentStr, "{{ .Rhdh.") &&
		!strings.Contains(contentStr, "{{- .Rhdh.") &&
		!strings.Contains(contentStr, "{{-if ") &&
		!strings.Contains(contentStr, "{{- if ") &&
		!strings.Contains(contentStr, "{{if ") {
		return content, nil
	}
	// If we found conditional patterns, verify they actually reference .Rhdh.
	// This prevents false positives from other template patterns
	if !strings.Contains(contentStr, ".Rhdh.") {
		return content, nil
	}
	tmpl, err := template.New("config").Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}
	return buf.Bytes(), nil
}
