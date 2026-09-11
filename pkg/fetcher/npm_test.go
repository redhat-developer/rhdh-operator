package fetcher

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Tests: parseNPMPackage()
// ============================================================================

func TestParseNPMPackage(t *testing.T) {
	tests := []struct {
		input           string
		expectedName    string
		expectedVersion string
	}{
		// Scoped packages with version
		{"@backstage/plugin-catalog@1.0.0", "@backstage/plugin-catalog", "1.0.0"},
		{"@scope/package@2.3.4", "@scope/package", "2.3.4"},

		// Unscoped packages with version
		{"lodash@4.17.21", "lodash", "4.17.21"},
		{"is-odd@3.0.1", "is-odd", "3.0.1"},

		// Without version (should return "latest")
		{"@backstage/plugin-catalog", "@backstage/plugin-catalog", "latest"},
		{"lodash", "lodash", "latest"},

		// With @npm: prefix
		{"@npm:@backstage/plugin@1.0.0", "@backstage/plugin", "1.0.0"},
		{"@npm:lodash@4.0.0", "lodash", "4.0.0"},

		// Edge cases
		{"@scope/pkg@0.0.1-beta.1", "@scope/pkg", "0.0.1-beta.1"},
		{"pkg@1.0.0-rc.1+build.123", "pkg", "1.0.0-rc.1+build.123"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, version := parseNPMPackage(tt.input)
			assert.Equal(t, tt.expectedName, name, "name mismatch")
			assert.Equal(t, tt.expectedVersion, version, "version mismatch")
		})
	}
}

// ============================================================================
// Tests: parseNPMRC()
// ============================================================================

func TestParseNPMRC(t *testing.T) {
	tests := []struct {
		name             string
		content          string
		expectedRegistry string
		expectedToken    string
	}{
		{
			name:             "default registry when file empty",
			content:          "",
			expectedRegistry: "",
			expectedToken:    "",
		},
		{
			name:             "custom registry",
			content:          "registry=https://npm.example.com",
			expectedRegistry: "https://npm.example.com",
			expectedToken:    "",
		},
		{
			name:             "registry with trailing slash preserved",
			content:          "registry=https://npm.example.com/",
			expectedRegistry: "https://npm.example.com/", // parseNPMRC returns raw value, trimming done in WithNPMConfigFile
			expectedToken:    "",
		},
		{
			name:             "auth token",
			content:          "//registry.npmjs.org/:_authToken=npm_MySecretToken123",
			expectedRegistry: "",
			expectedToken:    "npm_MySecretToken123",
		},
		{
			name: "registry and auth token",
			content: `registry=https://npm.private.com
//npm.private.com/:_authToken=secret_token_456`,
			expectedRegistry: "https://npm.private.com",
			expectedToken:    "secret_token_456",
		},
		{
			name: "quoted values",
			content: `registry="https://npm.quoted.com"
//npm.quoted.com/:_authToken="quoted_token"`,
			expectedRegistry: "https://npm.quoted.com",
			expectedToken:    "quoted_token",
		},
		{
			name: "single quoted values",
			content: `registry='https://npm.single.com'
//npm.single.com/:_authToken='single_token'`,
			expectedRegistry: "https://npm.single.com",
			expectedToken:    "single_token",
		},
		{
			name: "comments ignored",
			content: `# This is a comment
registry=https://npm.example.com
# Another comment
//npm.example.com/:_authToken=my_token`,
			expectedRegistry: "https://npm.example.com",
			expectedToken:    "my_token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp npmrc file
			tmpDir := t.TempDir()
			npmrcPath := filepath.Join(tmpDir, ".npmrc")
			err := os.WriteFile(npmrcPath, []byte(tt.content), 0644)
			require.NoError(t, err)

			registry, token := parseNPMRC(npmrcPath)
			assert.Equal(t, tt.expectedRegistry, registry, "registry mismatch")
			assert.Equal(t, tt.expectedToken, token, "token mismatch")
		})
	}
}

func TestParseNPMRC_FileNotFound(t *testing.T) {
	registry, token := parseNPMRC("/nonexistent/path/.npmrc")
	assert.Empty(t, registry)
	assert.Empty(t, token)
}

// ============================================================================
// Tests: verifyIntegrity()
// ============================================================================

func TestVerifyIntegrity(t *testing.T) {
	testData := []byte("hello world")

	// Pre-compute expected hashes
	sha256Hash := sha256.Sum256(testData)
	sha256Integrity := "sha256-" + base64.StdEncoding.EncodeToString(sha256Hash[:])

	sha512Hash := sha512.Sum512(testData)
	sha512Integrity := "sha512-" + base64.StdEncoding.EncodeToString(sha512Hash[:])

	sha384Hash := sha512.Sum384(testData)
	sha384Integrity := "sha384-" + base64.StdEncoding.EncodeToString(sha384Hash[:])

	tests := []struct {
		name      string
		data      []byte
		integrity string
		wantErr   bool
	}{
		{
			name:      "sha256 valid",
			data:      testData,
			integrity: sha256Integrity,
			wantErr:   false,
		},
		{
			name:      "sha512 valid",
			data:      testData,
			integrity: sha512Integrity,
			wantErr:   false,
		},
		{
			name:      "sha384 valid",
			data:      testData,
			integrity: sha384Integrity,
			wantErr:   false,
		},
		{
			name:      "sha256 invalid",
			data:      testData,
			integrity: "sha256-wronghashAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			wantErr:   true,
		},
		{
			name:      "unsupported algorithm",
			data:      testData,
			integrity: "md5-XUFAKrxLKna5cZ2REBfFkg==",
			wantErr:   true,
		},
		{
			name:      "invalid format - no dash",
			data:      testData,
			integrity: "sha256nohash",
			wantErr:   true,
		},
		{
			name:      "empty integrity fails format check",
			data:      testData,
			integrity: "", // Empty string checked at caller level (NPMFetcher.FetchWithIntegrity)
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyIntegrity(tt.data, tt.integrity)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ============================================================================
// Tests: NPMFetcher options
// ============================================================================

func TestNewNPMFetcher(t *testing.T) {
	f := NewNPMFetcher()
	assert.Equal(t, "https://registry.npmjs.org", f.registryURL)
	assert.Empty(t, f.authToken)
	assert.False(t, f.skipIntegrityCheck)
}

func TestWithNPMRegistry(t *testing.T) {
	f := NewNPMFetcher(WithNPMRegistry("https://npm.example.com/"))
	assert.Equal(t, "https://npm.example.com", f.registryURL) // trailing slash removed
}

func TestWithNPMAuthToken(t *testing.T) {
	f := NewNPMFetcher(WithNPMAuthToken("my-token"))
	assert.Equal(t, "my-token", f.authToken)
}

func TestWithSkipIntegrityCheck(t *testing.T) {
	f := NewNPMFetcher(WithSkipIntegrityCheck())
	assert.True(t, f.skipIntegrityCheck)
}

func TestWithNPMConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	npmrcPath := filepath.Join(tmpDir, ".npmrc")
	content := `registry=https://npm.private.com
//npm.private.com/:_authToken=file_token`
	err := os.WriteFile(npmrcPath, []byte(content), 0644)
	require.NoError(t, err)

	f := NewNPMFetcher(WithNPMConfigFile(npmrcPath))
	assert.Equal(t, "https://npm.private.com", f.registryURL)
	assert.Equal(t, "file_token", f.authToken)
}

// ============================================================================
// Tests: NPMFetcher with mock server
// ============================================================================

func TestNPMFetcher_ResolveLatestVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/is-odd" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"dist-tags": {
					"latest": "3.0.1"
				}
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	f := NewNPMFetcher(WithNPMRegistry(server.URL))
	version, err := f.resolveLatestVersion(context.Background(), "is-odd")
	require.NoError(t, err)
	assert.Equal(t, "3.0.1", version)
}

func TestNPMFetcher_GetVersionMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/is-odd/3.0.1" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"dist": {
					"tarball": "https://registry.npmjs.org/is-odd/-/is-odd-3.0.1.tgz",
					"integrity": "sha512-abc123=="
				}
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	f := NewNPMFetcher(WithNPMRegistry(server.URL))
	tarball, integrity, err := f.getVersionMetadata(context.Background(), "is-odd", "3.0.1")
	require.NoError(t, err)
	assert.Equal(t, "https://registry.npmjs.org/is-odd/-/is-odd-3.0.1.tgz", tarball)
	assert.Equal(t, "sha512-abc123==", integrity)
}

func TestNPMFetcher_AuthHeader(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"dist-tags": {"latest": "1.0.0"}}`))
	}))
	defer server.Close()

	f := NewNPMFetcher(
		WithNPMRegistry(server.URL),
		WithNPMAuthToken("my-secret-token"),
	)
	_, err := f.resolveLatestVersion(context.Background(), "test-pkg")
	require.NoError(t, err)
	assert.Equal(t, "Bearer my-secret-token", receivedAuth)
}

func TestNPMFetcher_404Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	f := NewNPMFetcher(WithNPMRegistry(server.URL))
	_, err := f.resolveLatestVersion(context.Background(), "nonexistent-pkg")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}
