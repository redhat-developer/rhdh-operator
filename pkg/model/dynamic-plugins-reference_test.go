package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		package_ string
		expected string
	}{
		{
			name:     "OCI with digest",
			package_: "oci://quay.io/rhdh/plugin@sha256:abc123",
			expected: "oci://quay.io/rhdh/plugin",
		},
		{
			name:     "OCI with tag",
			package_: "oci://quay.io/rhdh/plugin:v1.0.0",
			expected: "oci://quay.io/rhdh/plugin",
		},
		{
			name:     "OCI with digest and plugin path",
			package_: "oci://quay.io/rhdh/plugin@sha256:abc123!my-plugin",
			expected: "oci://quay.io/rhdh/plugin",
		},
		{
			name:     "OCI with tag and plugin path",
			package_: "oci://quay.io/rhdh/plugin:v1.0.0!my-plugin",
			expected: "oci://quay.io/rhdh/plugin",
		},
		{
			name:     "OCI with inherit suffix",
			package_: "oci://quay.io/rhdh/plugin:{{inherit}}",
			expected: "oci://quay.io/rhdh/plugin",
		},
		{
			name:     "OCI with inherit suffix and plugin path",
			package_: "oci://quay.io/rhdh/plugin:{{inherit}}!my-plugin",
			expected: "oci://quay.io/rhdh/plugin",
		},
		{
			name:     "OCI without tag or digest",
			package_: "oci://quay.io/rhdh/plugin",
			expected: "oci://quay.io/rhdh/plugin",
		},
		{
			name:     "Local path",
			package_: "./dynamic-plugins/dist/my-plugin",
			expected: "./dynamic-plugins/dist/my-plugin",
		},
		{
			name:     "NPM package",
			package_: "@backstage/plugin-catalog",
			expected: "@backstage/plugin-catalog",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := DynaPlugin{Package: tt.package_}
			result := plugin.BaseURL()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveInheritReference(t *testing.T) {
	baseURLMap := map[string]string{
		"oci://quay.io/rhdh/plugin-a":         "oci://quay.io/rhdh/plugin-a@sha256:abc123!plugin-a-path",
		"oci://quay.io/rhdh/plugin-b":         "oci://quay.io/rhdh/plugin-b@sha256:def456",
		"oci://registry.access.redhat.com/rh": "oci://registry.access.redhat.com/rh@sha256:xyz789!rh-plugin",
	}

	tests := []struct {
		name        string
		packageURL  string
		expected    string
		expectError bool
	}{
		{
			name:       "inherit without plugin path - uses full default",
			packageURL: "oci://quay.io/rhdh/plugin-a:{{inherit}}",
			expected:   "oci://quay.io/rhdh/plugin-a@sha256:abc123!plugin-a-path",
		},
		{
			name:       "inherit with plugin path - keeps user's plugin path",
			packageURL: "oci://quay.io/rhdh/plugin-a:{{inherit}}!custom-path",
			expected:   "oci://quay.io/rhdh/plugin-a@sha256:abc123!custom-path",
		},
		{
			name:       "inherit from base without plugin path",
			packageURL: "oci://quay.io/rhdh/plugin-b:{{inherit}}",
			expected:   "oci://quay.io/rhdh/plugin-b@sha256:def456",
		},
		{
			name:       "inherit from base without plugin path - user adds plugin path",
			packageURL: "oci://quay.io/rhdh/plugin-b:{{inherit}}!my-plugin",
			expected:   "oci://quay.io/rhdh/plugin-b@sha256:def456!my-plugin",
		},
		{
			name:        "inherit with no matching base - error",
			packageURL:  "oci://quay.io/rhdh/unknown:{{inherit}}",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveInheritReference(tt.packageURL, baseURLMap)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "cannot resolve {{inherit}} reference")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestResolveReferences(t *testing.T) {
	basePlugins := []DynaPlugin{
		{Package: "oci://quay.io/rhdh/plugin-a@sha256:abc123!plugin-a"},
		{Package: "oci://quay.io/rhdh/plugin-b@sha256:def456"},
		{Package: "./dynamic-plugins/dist/local-plugin"},
	}

	tests := []struct {
		name        string
		plugins     []DynaPlugin
		expected    []string
		expectError bool
	}{
		{
			name: "resolve single inherit reference",
			plugins: []DynaPlugin{
				{Package: "oci://quay.io/rhdh/plugin-a:{{inherit}}"},
			},
			expected: []string{"oci://quay.io/rhdh/plugin-a@sha256:abc123!plugin-a"},
		},
		{
			name: "resolve inherit with custom plugin path",
			plugins: []DynaPlugin{
				{Package: "oci://quay.io/rhdh/plugin-a:{{inherit}}!custom"},
			},
			expected: []string{"oci://quay.io/rhdh/plugin-a@sha256:abc123!custom"},
		},
		{
			name: "mixed - some inherit, some regular",
			plugins: []DynaPlugin{
				{Package: "oci://quay.io/rhdh/plugin-a:{{inherit}}"},
				{Package: "oci://quay.io/other/plugin@sha256:fixed"},
				{Package: "./local/path"},
			},
			expected: []string{
				"oci://quay.io/rhdh/plugin-a@sha256:abc123!plugin-a",
				"oci://quay.io/other/plugin@sha256:fixed",
				"./local/path",
			},
		},
		{
			name: "no references to resolve",
			plugins: []DynaPlugin{
				{Package: "oci://quay.io/plugin@sha256:123"},
				{Package: "./local"},
			},
			expected: []string{
				"oci://quay.io/plugin@sha256:123",
				"./local",
			},
		},
		{
			name: "inherit reference not found - error",
			plugins: []DynaPlugin{
				{Package: "oci://unknown/plugin:{{inherit}}"},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveReferences(tt.plugins, basePlugins)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(tt.expected), len(result))
				for i, pkg := range tt.expected {
					assert.Equal(t, pkg, result[i].Package)
				}
			}
		})
	}
}

func TestMergePluginsDataWithInherit(t *testing.T) {
	// Default config with versioned plugins
	defaultData := `
plugins:
  - package: "oci://quay.io/rhdh/plugin-a@sha256:abc123!plugin-a"
    disabled: false
    pluginConfig:
      key1: "value1"
  - package: "oci://quay.io/rhdh/plugin-b@sha256:def456"
    disabled: true
`

	// User config using inherit
	userData := `
plugins:
  - package: "oci://quay.io/rhdh/plugin-a:{{inherit}}"
    pluginConfig:
      key1: "overridden"
  - package: "oci://quay.io/rhdh/plugin-b:{{inherit}}!custom-path"
`

	mergedData, err := MergePluginsData(defaultData, userData)
	assert.NoError(t, err)
	assert.NotEmpty(t, mergedData)

	// Verify the inherit was resolved
	assert.Contains(t, mergedData, "oci://quay.io/rhdh/plugin-a@sha256:abc123!plugin-a")
	assert.Contains(t, mergedData, "oci://quay.io/rhdh/plugin-b@sha256:def456!custom-path")
	assert.NotContains(t, mergedData, "{{inherit}}")
}

func TestMergePluginsDataWithInheritError(t *testing.T) {
	defaultData := `
plugins:
  - package: "oci://quay.io/rhdh/plugin-a@sha256:abc123"
`

	// User config referencing non-existent plugin
	userData := `
plugins:
  - package: "oci://quay.io/rhdh/unknown-plugin:{{inherit}}"
`

	_, err := MergePluginsData(defaultData, userData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot resolve {{inherit}} reference")
}
