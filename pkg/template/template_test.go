package template

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyTemplate(t *testing.T) {
	// Create template data
	templateData := NewTemplateData(
		&MockBackstageCR{Name: "my-backstage", Namespace: "my-namespace"},
		&MockPlatform{Extension: "openshift"},
		&MockExternalConfig{IngressDomain: "apps.example.com"},
	)

	// Read YAML with template variables
	conf, err := os.ReadFile("testdata/configmap-template.yaml")
	require.NoError(t, err)

	// Apply templates
	templated, err := ApplyTemplate(templateData, conf)
	require.NoError(t, err)

	expected := `apiVersion: v1
kind: ConfigMap
metadata:
  name: config-my-backstage
  namespace: my-namespace
data:
  SERVICE_URL: "https://my-backstage.my-namespace.svc"
`
	assert.Equal(t, expected, string(templated))
}

func TestApplyTemplateSkipsNonRhdhPatterns(t *testing.T) {
	// Create template data
	templateData := NewTemplateData(
		&MockBackstageCR{Name: "my-backstage", Namespace: "my-namespace"},
		&MockPlatform{Extension: "kubernetes"},
		&MockExternalConfig{IngressDomain: ""},
	)

	tests := []struct {
		name    string
		content string
	}{
		{
			name: "prompt template with {{message}}",
			content: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  prompt: |
    Question: {{message}}
    Response: {{allowed}}
`,
		},
		{
			name: "python f-string with ${{message}}",
			content: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  script.py: |
    TEMPLATE = f"""
    Question:
    ${{message}}
    Response:
    """
`,
		},
		{
			name: "generic {{if}} without .Rhdh reference",
			content: `data:
  config: |
    {{if .Debug}}
    debug: true
    {{end}}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := []byte(tt.content)
			result, err := ApplyTemplate(templateData, content)
			assert.NoError(t, err)
			// Content should be unchanged since it doesn't contain .Rhdh. references
			assert.Equal(t, content, result)
		})
	}
}

func TestApplyTemplateWithNoDataSet(t *testing.T) {
	// Pass nil template data
	content := []byte(`name: {{.Rhdh.Name}}`)
	result, err := ApplyTemplate(nil, content)

	assert.NoError(t, err)
	// Should return unchanged
	assert.Equal(t, content, result)
}

func TestApplyTemplateWithRuntimeInfo(t *testing.T) {
	templateData := NewTemplateData(
		&MockBackstageCR{Name: "test-app", Namespace: "test-ns"},
		&MockPlatform{Extension: "openshift"},
		&MockExternalConfig{IngressDomain: "apps.example.com"},
	)

	content := []byte(`platform: {{.Rhdh.Runtime.Platform}}
domain: {{.Rhdh.Runtime.IngressDomain}}
url: https://{{.Rhdh.Name}}.{{.Rhdh.Runtime.IngressDomain}}`)

	result, err := ApplyTemplate(templateData, content)
	require.NoError(t, err)

	expected := `platform: openshift
domain: apps.example.com
url: https://test-app.apps.example.com`

	assert.Equal(t, expected, string(result))
}

func TestApplyTemplateWithConditionals(t *testing.T) {
	tests := []struct {
		name            string
		platform        string
		ingressDomain   string
		content         string
		expectedContent string
	}{
		{
			name:          "OCP platform includes conditional block",
			platform:      "ocp",
			ingressDomain: "apps.example.com",
			content: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  config.yaml: |
    base_config: true
    {{- if eq .Rhdh.Runtime.Platform "ocp"}}
    ocp_specific:
      enabled: true
      url: "http://service-{{.Rhdh.Name}}.{{.Rhdh.Runtime.IngressDomain}}"
    {{- end}}
    other_config: value`,
			expectedContent: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  config.yaml: |
    base_config: true
    ocp_specific:
      enabled: true
      url: "http://service-test-app.apps.example.com"
    other_config: value`,
		},
		{
			name:          "K8s platform excludes conditional block",
			platform:      "k8s",
			ingressDomain: "",
			content: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  config.yaml: |
    base_config: true
    {{- if eq .Rhdh.Runtime.Platform "ocp"}}
    ocp_specific:
      enabled: true
      url: "http://service-{{.Rhdh.Name}}.{{.Rhdh.Runtime.IngressDomain}}"
    {{- end}}
    other_config: value`,
			expectedContent: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  config.yaml: |
    base_config: true
    other_config: value`,
		},
		{
			name:          "Dynamic plugins with OCP dependency",
			platform:      "ocp",
			ingressDomain: "apps.example.com",
			content: `data:
  dynamic-plugins.yaml: |
    plugins:
      - package: ref://plugin-backend
        enabled: true
        {{- if eq .Rhdh.Runtime.Platform "ocp"}}
        dependencies:
          - ref: okp
        {{- end}}`,
			expectedContent: `data:
  dynamic-plugins.yaml: |
    plugins:
      - package: ref://plugin-backend
        enabled: true
        dependencies:
          - ref: okp`,
		},
		{
			name:          "Dynamic plugins without OCP dependency on k8s",
			platform:      "k8s",
			ingressDomain: "",
			content: `data:
  dynamic-plugins.yaml: |
    plugins:
      - package: ref://plugin-backend
        enabled: true
        {{- if eq .Rhdh.Runtime.Platform "ocp"}}
        dependencies:
          - ref: okp
        {{- end}}`,
			expectedContent: `data:
  dynamic-plugins.yaml: |
    plugins:
      - package: ref://plugin-backend
        enabled: true`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			templateData := NewTemplateData(
				&MockBackstageCR{Name: "test-app", Namespace: "test-ns"},
				&MockPlatform{Extension: tt.platform},
				&MockExternalConfig{IngressDomain: tt.ingressDomain},
			)

			result, err := ApplyTemplate(templateData, []byte(tt.content))
			require.NoError(t, err)

			assert.Equal(t, tt.expectedContent, string(result))
		})
	}
}

func TestApplyTemplateWithEscapedBraces(t *testing.T) {
	templateData := NewTemplateData(
		&MockBackstageCR{Name: "test-app", Namespace: "test-ns"},
		&MockPlatform{Extension: "k8s"},
		&MockExternalConfig{IngressDomain: ""},
	)

	// Simulates configmap-files.yaml with both .Rhdh. conditionals and escaped {{message}}
	content := `apiVersion: v1
kind: ConfigMap
data:
  config.yaml: |
    base_config: true
    {{- if eq .Rhdh.Runtime.Platform "ocp"}}
    ocp_only: true
    {{- end}}
  script.py: |
    TEMPLATE = f"""
    Question:
    ${{ "{{" }}message{{ "}}" }}
    Response:
    """
`

	result, err := ApplyTemplate(templateData, []byte(content))
	require.NoError(t, err)

	expected := `apiVersion: v1
kind: ConfigMap
data:
  config.yaml: |
    base_config: true
  script.py: |
    TEMPLATE = f"""
    Question:
    ${{message}}
    Response:
    """
`
	assert.Equal(t, expected, string(result))
}
