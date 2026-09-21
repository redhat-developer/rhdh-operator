package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyMirror(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		mirrors  *ImageDigestMirrors
		expected string
	}{
		{
			name:     "no mirrors configured",
			ref:      "oci://quay.io/rhdh/plugin:1.0",
			mirrors:  nil,
			expected: "oci://quay.io/rhdh/plugin:1.0",
		},
		{
			name: "empty mirrors list",
			ref:  "oci://quay.io/rhdh/plugin:1.0",
			mirrors: &ImageDigestMirrors{
				ImageDigestMirrors: []ImageDigestMirror{},
			},
			expected: "oci://quay.io/rhdh/plugin:1.0",
		},
		{
			name: "non-OCI URL not transformed",
			ref:  "https://example.com/plugin.tgz",
			mirrors: &ImageDigestMirrors{
				ImageDigestMirrors: []ImageDigestMirror{
					{Source: "example.com", Mirrors: []string{"mirror.example.com"}},
				},
			},
			expected: "https://example.com/plugin.tgz",
		},
		{
			name: "npm package not transformed",
			ref:  "@scope/package@1.0.0",
			mirrors: &ImageDigestMirrors{
				ImageDigestMirrors: []ImageDigestMirror{
					{Source: "registry.npmjs.org", Mirrors: []string{"npm-mirror.example.com"}},
				},
			},
			expected: "@scope/package@1.0.0",
		},
		{
			name: "simple mirror transformation",
			ref:  "oci://quay.io/rhdh/plugin:1.0",
			mirrors: &ImageDigestMirrors{
				ImageDigestMirrors: []ImageDigestMirror{
					{Source: "quay.io", Mirrors: []string{"mirror.example.com/quay"}},
				},
			},
			expected: "oci://mirror.example.com/quay/rhdh/plugin:1.0",
		},
		{
			name: "most specific source wins",
			ref:  "oci://quay.io/rhdh/plugin:1.0",
			mirrors: &ImageDigestMirrors{
				ImageDigestMirrors: []ImageDigestMirror{
					{Source: "quay.io", Mirrors: []string{"mirror1.example.com/quay"}},
					{Source: "quay.io/rhdh", Mirrors: []string{"mirror2.example.com/rhdh"}},
				},
			},
			expected: "oci://mirror2.example.com/rhdh/plugin:1.0",
		},
		{
			name: "no matching source",
			ref:  "oci://registry.redhat.io/rhdh/plugin:1.0",
			mirrors: &ImageDigestMirrors{
				ImageDigestMirrors: []ImageDigestMirror{
					{Source: "quay.io", Mirrors: []string{"mirror.example.com/quay"}},
				},
			},
			expected: "oci://registry.redhat.io/rhdh/plugin:1.0",
		},
		{
			name: "mirror with digest",
			ref:  "oci://quay.io/rhdh/plugin@sha256:abc123",
			mirrors: &ImageDigestMirrors{
				ImageDigestMirrors: []ImageDigestMirror{
					{Source: "quay.io", Mirrors: []string{"mirror.example.com/quay"}},
				},
			},
			expected: "oci://mirror.example.com/quay/rhdh/plugin@sha256:abc123",
		},
		{
			name: "multiple mirrors - uses first",
			ref:  "oci://quay.io/rhdh/plugin:1.0",
			mirrors: &ImageDigestMirrors{
				ImageDigestMirrors: []ImageDigestMirror{
					{Source: "quay.io", Mirrors: []string{"mirror1.example.com/quay", "mirror2.example.com/quay"}},
				},
			},
			expected: "oci://mirror1.example.com/quay/rhdh/plugin:1.0",
		},
		{
			name: "mirror source without mirrors list",
			ref:  "oci://quay.io/rhdh/plugin:1.0",
			mirrors: &ImageDigestMirrors{
				ImageDigestMirrors: []ImageDigestMirror{
					{Source: "quay.io", Mirrors: []string{}},
				},
			},
			expected: "oci://quay.io/rhdh/plugin:1.0",
		},
		{
			name: "exact source match",
			ref:  "oci://registry.access.redhat.com/rhdh/plugin:1.0",
			mirrors: &ImageDigestMirrors{
				ImageDigestMirrors: []ImageDigestMirror{
					{Source: "registry.access.redhat.com", Mirrors: []string{"internal-mirror.example.com/redhat"}},
				},
			},
			expected: "oci://internal-mirror.example.com/redhat/rhdh/plugin:1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyMirror(tt.ref, tt.mirrors)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestApplyMirrorPerformance(t *testing.T) {
	// Test that early return works correctly for nil mirrors
	result := ApplyMirror("oci://quay.io/rhdh/plugin:1.0", nil)
	assert.Equal(t, "oci://quay.io/rhdh/plugin:1.0", result)

	// Test that early return works for empty mirrors
	emptyMirrors := &ImageDigestMirrors{ImageDigestMirrors: []ImageDigestMirror{}}
	result = ApplyMirror("oci://quay.io/rhdh/plugin:1.0", emptyMirrors)
	assert.Equal(t, "oci://quay.io/rhdh/plugin:1.0", result)
}

func TestReadMirrorConfigFromFile(t *testing.T) {
	tests := []struct {
		name         string
		fileContent  string
		expectError  bool
		expectNil    bool
		validateFunc func(*testing.T, interface{})
	}{
		{
			name: "valid mirror config with multiple sources",
			fileContent: `imageDigestMirrors:
  - source: quay.io
    mirrors:
      - mirror.example.com/quay
  - source: registry.redhat.io
    mirrors:
      - mirror.example.com/redhat
`,
			expectError: false,
			expectNil:   false,
			validateFunc: func(t *testing.T, result interface{}) {
				mirrors := result.(*ImageDigestMirrors)
				assert.Len(t, mirrors.ImageDigestMirrors, 2)
				assert.Equal(t, "quay.io", mirrors.ImageDigestMirrors[0].Source)
				assert.Equal(t, []string{"mirror.example.com/quay"}, mirrors.ImageDigestMirrors[0].Mirrors)
				assert.Equal(t, "registry.redhat.io", mirrors.ImageDigestMirrors[1].Source)
			},
		},
		{
			name: "empty mirrors list",
			fileContent: `imageDigestMirrors: []
`,
			expectError: false,
			expectNil:   false,
			validateFunc: func(t *testing.T, result interface{}) {
				mirrors := result.(*ImageDigestMirrors)
				assert.Empty(t, mirrors.ImageDigestMirrors)
			},
		},
		{
			name:        "invalid yaml",
			fileContent: `invalid: yaml: content: [unclosed`,
			expectError: true,
			expectNil:   false,
		},
		{
			name:        "empty file",
			fileContent: "",
			expectError: false,
			expectNil:   false,
			validateFunc: func(t *testing.T, result interface{}) {
				mirrors := result.(*ImageDigestMirrors)
				assert.Empty(t, mirrors.ImageDigestMirrors)
			},
		},
		{
			name: "mirror with multiple fallback mirrors",
			fileContent: `imageDigestMirrors:
  - source: ghcr.io
    mirrors:
      - primary-mirror.example.com/ghcr
      - backup-mirror.example.com/ghcr
`,
			expectError: false,
			expectNil:   false,
			validateFunc: func(t *testing.T, result interface{}) {
				mirrors := result.(*ImageDigestMirrors)
				assert.Len(t, mirrors.ImageDigestMirrors, 1)
				assert.Equal(t, "ghcr.io", mirrors.ImageDigestMirrors[0].Source)
				assert.Equal(t, []string{"primary-mirror.example.com/ghcr", "backup-mirror.example.com/ghcr"}, mirrors.ImageDigestMirrors[0].Mirrors)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpDir := t.TempDir()
			testFile := filepath.Join(tmpDir, "mirrors.yaml")

			err := os.WriteFile(testFile, []byte(tt.fileContent), 0644)
			require.NoError(t, err)

			// Test readMirrorConfigFromFile
			mirrors, err := readMirrorConfigFromFile(testFile)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectNil {
				assert.Nil(t, mirrors)
			} else if !tt.expectError {
				assert.NotNil(t, mirrors)
				if tt.validateFunc != nil {
					tt.validateFunc(t, mirrors)
				}
			}
		})
	}
}

func TestReadMirrorConfigFromFile_FileNotExists(t *testing.T) {
	// Test with non-existent file
	nonExistentFile := "/tmp/definitely-does-not-exist-mirror-test.yaml"

	mirrors, err := readMirrorConfigFromFile(nonExistentFile)

	assert.NoError(t, err, "should not return error when file doesn't exist")
	assert.Nil(t, mirrors, "should return nil when file doesn't exist")
}

func TestGetMirrorConfig(t *testing.T) {
	// Test the public function with the default path
	// This will return nil if the file doesn't exist (expected in test environment)
	_, err := GetMirrorConfig()

	assert.NoError(t, err, "should not error when default config file doesn't exist")
	// mirrors will be nil in test env where /plugins-mirror/mirrors.yaml doesn't exist
	// In production with ConfigMap mounted, it would return the config
}
