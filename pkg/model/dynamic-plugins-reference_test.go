package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestResolveInheritReference(t *testing.T) {
	basePlugins := []DynaPlugin{
		{Package: "oci://quay.io/rhdh/plugin-a@sha256:abc123!plugin-a"},
		{Package: "oci://quay.io/rhdh/plugin-b@sha256:def456"},
		{Package: "oci://registry.access.redhat.com/plugin-c@sha256:xyz789!plugin-c"},
		{Package: "./dynamic-plugins/dist/plugin-local"},
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
			expected:   "oci://quay.io/rhdh/plugin-a@sha256:abc123!plugin-a",
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
			expected:   "oci://registry.access.redhat.com/plugin-c@sha256:xyz789!plugin-c",
		},
		{
			name:        "inherit with no matching base - error",
			packageURL:  "oci://quay.io/rhdh/unknown:{{inherit}}",
			expectError: true,
		},
		{
			name:        "inherit from local base plugin - error",
			packageURL:  "oci://quay.io/rhdh/plugin-local:{{inherit}}",
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
		{
			name: "ref to local base plugin - error",
			plugins: []DynaPlugin{
				{Package: "ref://local-plugin"},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveReferences(tt.plugins, basePlugins, false)

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

func TestMergePluginsDataWithInlineCatalogReferences(t *testing.T) {
	const base = `
plugins:
  - package: oci://quay.io/rhdh/plugin-a@sha256:abc123
    enabled: false
    integrity: sha256-catalog
    pluginConfig:
      title: catalog
`
	for _, mode := range []string{"false", "true"} {
		for _, reference := range []string{"ref://plugin-a", "oci://quay.io/rhdh/plugin-a:{{inherit}}"} {
			t.Run(mode+"/"+reference, func(t *testing.T) {
				t.Setenv(OperatorDPProcessingEnvVar, mode)
				overlay := "plugins:\n  - package: " + reference + "\n    enabled: true\n    pluginConfig:\n      title: flavour\n"
				merged, err := MergePluginsData(base, overlay)
				require.NoError(t, err)

				var config DynaPluginsConfig
				require.NoError(t, yaml.Unmarshal([]byte(merged), &config))
				require.Len(t, config.Plugins, 1, "the flavour must override the catalog entry rather than add an unresolved duplicate")
				plugin := config.Plugins[0]
				assert.Equal(t, "oci://quay.io/rhdh/plugin-a@sha256:abc123", plugin.Package)
				assert.False(t, plugin.IsDisabled())
				assert.Equal(t, "sha256-catalog", plugin.Integrity)
				assert.Equal(t, "flavour", plugin.PluginConfig["title"])
			})
		}
	}
}

func TestMergePluginsDataWithInstallerReferences(t *testing.T) {
	const base = `
includes:
  - dynamic-plugins.default.yaml
plugins:
  - package: oci://quay.io/rhdh/plugin-a@sha256:abc123
    enabled: false
`
	const overlay = `
plugins:
  - package: ref://plugin-a
    enabled: true
  - package: ref://included-plugin
    enabled: true
  - package: oci://quay.io/rhdh/another-included-plugin:{{inherit}}
    enabled: true
  - package: '@example/npm-plugin'
    enabled: true
`
	t.Run("installer resolves references from includes", func(t *testing.T) {
		t.Setenv(OperatorDPProcessingEnvVar, "false")
		merged, err := MergePluginsData(base, overlay)
		require.NoError(t, err)

		var config DynaPluginsConfig
		require.NoError(t, yaml.Unmarshal([]byte(merged), &config))
		assert.Equal(t, []string{"dynamic-plugins.default.yaml"}, config.Includes)
		require.Len(t, config.Plugins, 4)
		plugin := findPluginByPackage(config.Plugins, "oci://quay.io/rhdh/plugin-a@sha256:abc123")
		require.NotNil(t, plugin)
		assert.False(t, plugin.IsDisabled())
		assert.NotNil(t, findPluginByPackage(config.Plugins, "ref://included-plugin"))
		assert.NotNil(t, findPluginByPackage(config.Plugins, "oci://quay.io/rhdh/another-included-plugin:{{inherit}}"))
		assert.NotNil(t, findPluginByPackage(config.Plugins, "@example/npm-plugin"))
	})
	t.Run("operator processing requires references to resolve", func(t *testing.T) {
		t.Setenv(OperatorDPProcessingEnvVar, "true")
		_, err := MergePluginsData(base, overlay)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot resolve ref:// reference")
	})
	t.Run("installer mode still rejects invalid references", func(t *testing.T) {
		t.Setenv(OperatorDPProcessingEnvVar, "false")
		_, err := MergePluginsData(base, "plugins:\n  - package: ref://\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty plugin name")
	})
}

func TestMergePluginsDataWithNonOCIInlineMatch(t *testing.T) {
	for _, mode := range []string{"false", "true"} {
		for _, packageURL := range []string{"https://example.com/plugin-a-1.0.0.tgz", "./dynamic-plugins/plugin-a"} {
			for _, reference := range []string{"ref://plugin-a", "oci://quay.io/rhdh/plugin-a:{{inherit}}"} {
				t.Run(mode+"/"+packageURL+"/"+reference, func(t *testing.T) {
					t.Setenv(OperatorDPProcessingEnvVar, mode)
					base := "plugins:\n  - package: " + packageURL + "\n    enabled: false\n"
					overlay := "includes:\n  - included.yaml\nplugins:\n  - package: " + reference + "\n    enabled: true\n"
					merged, err := MergePluginsData(base, overlay)
					if mode == "true" {
						require.Error(t, err, "Operator processing must keep its OCI-only reference restriction")
						assert.Contains(t, err.Error(), "non-OCI")
						return
					}
					require.NoError(t, err, "a non-OCI inline entry must not prevent the installer from resolving an included OCI plugin")

					var config DynaPluginsConfig
					require.NoError(t, yaml.Unmarshal([]byte(merged), &config))
					assert.Equal(t, []string{"included.yaml"}, config.Includes)
					require.Len(t, config.Plugins, 2)
					inline := findPluginByPackage(config.Plugins, packageURL)
					require.NotNil(t, inline)
					assert.True(t, inline.IsDisabled(), "the non-OCI inline plugin must remain disabled")
					deferred := findPluginByPackage(config.Plugins, reference)
					require.NotNil(t, deferred, "preserve the reference unchanged for the installer")
					assert.False(t, deferred.IsDisabled())
				})
			}
		}
	}
}

func TestResolveRefReference(t *testing.T) {
	basePlugins := []DynaPlugin{
		{Package: "oci://quay.io/rhdh/backstage-plugin-foo@sha256:abc123"},
		{Package: "oci://quay.io/rhdh/backstage-plugin-bar@sha256:def456!backstage-plugin-bar"},
		{Package: "https://example.com/plugins/backstage-plugin-https-1.0.0.tgz"},
		{Package: "./dynamic-plugins/dist/backstage-plugin-local"},
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
			expected:   "oci://quay.io/rhdh/backstage-plugin-bar@sha256:def456!backstage-plugin-bar",
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
		{
			name:        "ref to HTTPS base plugin - error",
			packageURL:  "ref://backstage-plugin-https",
			expectError: true,
		},
		{
			name:        "ref to local base plugin - error",
			packageURL:  "ref://backstage-plugin-local",
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
			name:     "OCI with tag and digest",
			package_: "oci://quay.io/rhdh/backstage-plugin-baz:v1.0.0@sha256:abc123",
			expected: "backstage-plugin-baz",
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
		{
			name:     "OCI with pluginPath suffix",
			package_: "oci://quay.io/rhdh/multi-plugin:1.0!plugin-A",
			expected: "plugin-A",
		},
		{
			name:     "OCI with digest and pluginPath",
			package_: "oci://quay.io/rhdh/multi-plugin@sha256:abc123!my-plugin",
			expected: "my-plugin",
		},
		{
			name:     "HTTPS with pluginPath suffix",
			package_: "https://example.com/plugins/multi-1.0.0.tgz!plugin-B",
			expected: "plugin-B",
		},
		{
			name:     "Local path with pluginPath suffix",
			package_: "./dynamic-plugins/dist/multi-plugin!specific-plugin",
			expected: "specific-plugin",
		},
		{
			name:     "OCI with pluginPath containing exclamation mark",
			package_: "oci://quay.io/rhdh/multi-plugin:1.0!plugin-v2!beta",
			expected: "plugin-v2!beta",
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
