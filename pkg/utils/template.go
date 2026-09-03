package utils

import (
	"bytes"
	"fmt"
	"text/template"
)

// TemplateData provides values for Go template substitution in config files.
// Use {{.Backstage.Name}} and {{.Backstage.Namespace}} in YAML files.
type TemplateData struct {
	Backstage BackstageInfo
}

// BackstageInfo contains Backstage CR fields available for templating in config files.
type BackstageInfo struct {
	Name      string
	Namespace string
}

// templateData holds the current template data for YAML processing.
var templateData *TemplateData

// SetTemplateData sets the template data for YAML file processing.
// Call this once before reading config files.
func SetTemplateData(name, namespace string) {
	templateData = &TemplateData{
		Backstage: BackstageInfo{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// ApplyTemplate applies Go template substitution to content if templateData is set.
// Returns content unchanged if no template data has been set.
func ApplyTemplate(content []byte) ([]byte, error) {
	if templateData == nil {
		return content, nil
	}
	tmpl, err := template.New("config").Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}
	return buf.Bytes(), nil
}
