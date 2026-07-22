package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveInheritReference(t *testing.T) {
	basePlugins := []DynaPlugin{
		{Package: "oci://quay.io/rhdh/plugin-a@sha256:abc123!plugin-a-path"},
		{Package: "oci://quay.io/rhdh/plugin-b@sha256:def456"},
		{Package: "oci://registry.access.redhat.com/plugin-c@sha256:xyz789!rh-plugin"},
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
			name:       "inherit with different registry - matches by name",
			packageURL: "oci://other-registry.io/different/plugin-c:{{inherit}}",
			expected:   "oci://registry.access.redhat.com/plugin-c@sha256:xyz789!rh-plugin",
		},
		{
			name:        "inherit with no matching base - error",
			packageURL:  "oci://quay.io/rhdh/unknown:{{inherit}}",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveInheritReference(tt.packageURL, basePlugins)

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
		{
			name: "unsupported protocol - error",
			plugins: []DynaPlugin{
				{Package: "ftp://server.example.com/plugin"},
			},
			expectError: true,
		},
		{
			name: "npm package not supported - error",
			plugins: []DynaPlugin{
				{Package: "@backstage/plugin-catalog"},
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
	t.Setenv(OperatorDPProcessingEnvVar, "true")

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

func TestResolveRefReference(t *testing.T) {
	basePlugins := []DynaPlugin{
		{Package: "oci://quay.io/rhdh/backstage-plugin-foo@sha256:abc123"},
		{Package: "oci://quay.io/rhdh/backstage-plugin-bar@sha256:def456!plugin-path"},
	}

	tests := []struct {
		name        string
		packageURL  string
		expected    string
		expectError bool
	}{
		{
			name:       "ref to OCI plugin",
			packageURL: "ref://backstage-plugin-foo",
			expected:   "oci://quay.io/rhdh/backstage-plugin-foo@sha256:abc123",
		},
		{
			name:       "ref to OCI plugin with path",
			packageURL: "ref://backstage-plugin-bar",
			expected:   "oci://quay.io/rhdh/backstage-plugin-bar@sha256:def456!plugin-path",
		},
		{
			name:        "ref to non-existent plugin",
			packageURL:  "ref://unknown-plugin",
			expectError: true,
		},
		{
			name:        "empty ref",
			packageURL:  "ref://",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveRefReference(tt.packageURL, basePlugins)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestName(t *testing.T) {
	tests := []struct {
		name     string
		package_ string
		expected string
	}{
		{
			name:     "OCI with digest",
			package_: "oci://quay.io/rhdh/backstage-plugin-foo@sha256:abc123",
			expected: "backstage-plugin-foo",
		},
		{
			name:     "OCI with tag",
			package_: "oci://quay.io/rhdh/backstage-plugin-bar:v1.0.0",
			expected: "backstage-plugin-bar",
		},
		{
			name:     "OCI with registry port and no tag",
			package_: "oci://localhost:5000/path/my-plugin",
			expected: "my-plugin",
		},
		{
			name:     "OCI with registry port and tag",
			package_: "oci://localhost:5000/path/my-plugin:v1.0.0",
			expected: "my-plugin",
		},
		{
			name:     "OCI registry-only URL returns empty",
			package_: "oci://localhost:5000",
			expected: "",
		},
		{
			name:     "OCI registry with trailing slash returns empty",
			package_: "oci://localhost:5000/",
			expected: "",
		},
		{
			name:     "HTTPS with version and tgz",
			package_: "https://example.com/plugins/backstage-plugin-foo-1.0.0.tgz",
			expected: "backstage-plugin-foo",
		},
		{
			name:     "HTTPS with tar.gz",
			package_: "https://example.com/path/my-plugin-2.3.4.tar.gz",
			expected: "my-plugin",
		},
		{
			name:     "HTTP URL",
			package_: "http://registry.example.com/backstage-plugin-bar-0.1.0.tgz",
			expected: "backstage-plugin-bar",
		},
		{
			name:     "Local path",
			package_: "./dynamic-plugins/dist/backstage-plugin-techdocs",
			expected: "backstage-plugin-techdocs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := DynaPlugin{Package: tt.package_}
			result := plugin.Name()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMergePluginsDataWithInheritError(t *testing.T) {
	t.Setenv(OperatorDPProcessingEnvVar, "true")

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
