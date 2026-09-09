package fetcher

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOCIFetcher(t *testing.T) {
	fetcher := NewOCIFetcher()
	assert.NotNil(t, fetcher)
	assert.NotNil(t, fetcher.keychain)
	assert.NotNil(t, fetcher.transport)
	assert.False(t, fetcher.pluginMode)
}

func TestWithInsecure(t *testing.T) {
	fetcher := NewOCIFetcher(WithInsecure())

	transport, ok := fetcher.transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.TLSClientConfig)
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
}

func TestWithCACert(t *testing.T) {
	// Valid self-signed CA cert for testing
	caCert := []byte(`-----BEGIN CERTIFICATE-----
MIIDAzCCAeugAwIBAgIUIzY7ufM5PVWGCBw22V3JZPNkhTwwDQYJKoZIhvcNAQEL
BQAwETEPMA0GA1UEAwwGdGVzdGNhMB4XDTI2MDkwODA3NTczN1oXDTI3MDkwODA3
NTczN1owETEPMA0GA1UEAwwGdGVzdGNhMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A
MIIBCgKCAQEAysJqbjbazjOtfFGwyju4iL3GPpV7EiLHC+ylqssjtEOeOmH7x/nL
lakUw8d5wNerMtsha6L6CXvrloKbNgGMmZ6XNrvXcgHzxAq8Lm62Gm+6jc/M3ASV
uEuMLOAdwvuBrzZpeMwrpBSgBSETkKIcrLGiUbCW14wRjkM9xdtSKrzAwlvp0AIh
mm0N2H+AI8wi23VBmqP/dlh6PxqIUxNftPqputee+dOPDyoRL41TrYhruTCTViU5
p9Lw7fkCXeP5p7m5B4b6Fe/HZxqz3DLukXFlRjUtmX/9xwrLZFV15NZDRZ94LIpw
EGzeZcSyqZKNSUnfoQtmFph5br505XiNfwIDAQABo1MwUTAdBgNVHQ4EFgQU9136
wHKbv30UeC7Syauw58dlDRowHwYDVR0jBBgwFoAU9136wHKbv30UeC7Syauw58dl
DRowDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEATSuRRn2bfs23
Ixg9AOjdmrtNciWJ4lWZsbBmxkMSfhbvKgwatbvZUINdeF2MHxP0Ne/Omb9ToHpi
4G3+BSHAB0V4gBIgti/vQI/R5OfW/f5xu1VgEeLZ/f5FTIMlxtcHr1ybbaxg2e8j
UPls5wo0I4FlxCe2qLRWP2xiHSB/UwnjUjbw/UGEd6q2vKaDozuHWQctojRxDi1t
grubJzad+j1FeXEDmJuM7ZN91U5oEbycHH1Nj7RYQGWjT6i2mXIijorvb5WjOKUp
uSe02gSkNpStED1tN4YzA7z1S/tyLw8rQeAQL3CfAz9tMZHcv6cW0nkCMXpNNJc7
xEeZn/ELwg==
-----END CERTIFICATE-----`)

	fetcher := NewOCIFetcher(WithCACert(caCert))

	transport, ok := fetcher.transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.TLSClientConfig)
	assert.NotNil(t, transport.TLSClientConfig.RootCAs)
}

func TestWithCACert_Invalid(t *testing.T) {
	// Invalid CA cert - should fall back to default transport
	invalidCert := []byte(`-----BEGIN CERTIFICATE-----
INVALID_CERTIFICATE_DATA
-----END CERTIFICATE-----`)

	fetcher := NewOCIFetcher(WithCACert(invalidCert))

	// Should fall back to default transport (not set custom transport)
	assert.Equal(t, http.DefaultTransport, fetcher.transport)
}

func TestWithPluginValidation(t *testing.T) {
	fetcher := NewOCIFetcher(WithPluginValidation())
	assert.True(t, fetcher.validatePlugin)
}

func TestWithDockerConfig(t *testing.T) {
	dockerConfig := createDockerConfig(t, map[string]authEntry{
		"registry.example.com": {Username: "user", Password: "pass"},
	})

	fetcher := NewOCIFetcher(WithDockerConfig(dockerConfig))
	assert.NotNil(t, fetcher.keychain)

	// Verify we can resolve credentials
	kc, ok := fetcher.keychain.(*dockerConfigKeychain)
	require.True(t, ok)
	assert.Len(t, kc.auths, 1)
}

func TestDockerConfigKeychain_Resolve(t *testing.T) {
	tests := []struct {
		name         string
		auths        map[string]authEntry
		registry     string
		expectedUser string
		expectedAnon bool
	}{
		{
			name: "exact match",
			auths: map[string]authEntry{
				"registry.example.com": {Username: "user1", Password: "pass1"},
			},
			registry:     "registry.example.com",
			expectedUser: "user1",
		},
		{
			name: "https prefix match",
			auths: map[string]authEntry{
				"https://registry.example.com": {Username: "user2", Password: "pass2"},
			},
			registry:     "registry.example.com",
			expectedUser: "user2",
		},
		{
			name: "docker hub style v1 suffix",
			auths: map[string]authEntry{
				"https://index.docker.io/v1/": {Username: "user3", Password: "pass3"},
			},
			registry:     "index.docker.io",
			expectedUser: "user3",
		},
		{
			name: "no match returns anonymous",
			auths: map[string]authEntry{
				"other.registry.com": {Username: "user", Password: "pass"},
			},
			registry:     "registry.example.com",
			expectedAnon: true,
		},
		{
			name: "base64 auth field",
			auths: map[string]authEntry{
				"registry.example.com": {Auth: base64.StdEncoding.EncodeToString([]byte("user4:pass4"))},
			},
			registry:     "registry.example.com",
			expectedUser: "user4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dockerConfig := createDockerConfig(t, tt.auths)
			kc, err := newDockerConfigKeychain(dockerConfig)
			require.NoError(t, err)

			ref, err := name.ParseReference(tt.registry + "/test/image:latest")
			require.NoError(t, err)

			auth, err := kc.Resolve(ref.Context().Registry)
			require.NoError(t, err)

			if tt.expectedAnon {
				assert.Equal(t, authn.Anonymous, auth)
			} else {
				cfg, err := auth.Authorization()
				require.NoError(t, err)
				assert.Equal(t, tt.expectedUser, cfg.Username)
			}
		})
	}
}

func TestNewDockerConfigKeychain_InvalidJSON(t *testing.T) {
	_, err := newDockerConfigKeychain([]byte("not valid json"))
	assert.Error(t, err)
}

func TestAuthToAuthenticator_EmptyAuth(t *testing.T) {
	auth, err := authToAuthenticator(dockerAuthConfig{})
	require.NoError(t, err)
	assert.Equal(t, authn.Anonymous, auth)
}

func TestAuthToAuthenticator_InvalidBase64(t *testing.T) {
	_, err := authToAuthenticator(dockerAuthConfig{Auth: "not-valid-base64!"})
	assert.Error(t, err)
}

func TestAuthToAuthenticator_MalformedAuthString(t *testing.T) {
	// Base64 encoded "no-colon-here"
	auth, err := authToAuthenticator(dockerAuthConfig{
		Auth: base64.StdEncoding.EncodeToString([]byte("nocolonhere")),
	})
	require.NoError(t, err)
	// Should return anonymous when format is wrong (no colon separator)
	assert.Equal(t, authn.Anonymous, auth)
}

func TestMultipleOptions(t *testing.T) {
	caCert := []byte(`-----BEGIN CERTIFICATE-----
MIIDAzCCAeugAwIBAgIUIzY7ufM5PVWGCBw22V3JZPNkhTwwDQYJKoZIhvcNAQEL
BQAwETEPMA0GA1UEAwwGdGVzdGNhMB4XDTI2MDkwODA3NTczN1oXDTI3MDkwODA3
NTczN1owETEPMA0GA1UEAwwGdGVzdGNhMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A
MIIBCgKCAQEAysJqbjbazjOtfFGwyju4iL3GPpV7EiLHC+ylqssjtEOeOmH7x/nL
lakUw8d5wNerMtsha6L6CXvrloKbNgGMmZ6XNrvXcgHzxAq8Lm62Gm+6jc/M3ASV
uEuMLOAdwvuBrzZpeMwrpBSgBSETkKIcrLGiUbCW14wRjkM9xdtSKrzAwlvp0AIh
mm0N2H+AI8wi23VBmqP/dlh6PxqIUxNftPqputee+dOPDyoRL41TrYhruTCTViU5
p9Lw7fkCXeP5p7m5B4b6Fe/HZxqz3DLukXFlRjUtmX/9xwrLZFV15NZDRZ94LIpw
EGzeZcSyqZKNSUnfoQtmFph5br505XiNfwIDAQABo1MwUTAdBgNVHQ4EFgQU9136
wHKbv30UeC7Syauw58dlDRowHwYDVR0jBBgwFoAU9136wHKbv30UeC7Syauw58dl
DRowDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEATSuRRn2bfs23
Ixg9AOjdmrtNciWJ4lWZsbBmxkMSfhbvKgwatbvZUINdeF2MHxP0Ne/Omb9ToHpi
4G3+BSHAB0V4gBIgti/vQI/R5OfW/f5xu1VgEeLZ/f5FTIMlxtcHr1ybbaxg2e8j
UPls5wo0I4FlxCe2qLRWP2xiHSB/UwnjUjbw/UGEd6q2vKaDozuHWQctojRxDi1t
grubJzad+j1FeXEDmJuM7ZN91U5oEbycHH1Nj7RYQGWjT6i2mXIijorvb5WjOKUp
uSe02gSkNpStED1tN4YzA7z1S/tyLw8rQeAQL3CfAz9tMZHcv6cW0nkCMXpNNJc7
xEeZn/ELwg==
-----END CERTIFICATE-----`)

	dockerConfig := createDockerConfig(t, map[string]authEntry{
		"registry.example.com": {Username: "user", Password: "pass"},
	})

	// Apply multiple options - last one that sets transport wins for transport
	fetcher := NewOCIFetcher(
		WithCACert(caCert),
		WithDockerConfig(dockerConfig),
		WithPluginMode(),
	)

	// Verify all options were applied
	assert.True(t, fetcher.pluginMode)

	transport, ok := fetcher.transport.(*http.Transport)
	require.True(t, ok)
	assert.NotNil(t, transport.TLSClientConfig)

	kc, ok := fetcher.keychain.(*dockerConfigKeychain)
	require.True(t, ok)
	assert.Len(t, kc.auths, 1)
}

func TestWithDockerConfig_Invalid(t *testing.T) {
	// Invalid JSON - should fall back to default keychain
	invalidConfig := []byte(`not valid json`)

	fetcher := NewOCIFetcher(WithDockerConfig(invalidConfig))

	// Should fall back to default keychain
	assert.Equal(t, authn.DefaultKeychain, fetcher.keychain)
}

func TestInsecureOverridesCACert(t *testing.T) {
	caCert := []byte(`-----BEGIN CERTIFICATE-----
MIIDAzCCAeugAwIBAgIUIzY7ufM5PVWGCBw22V3JZPNkhTwwDQYJKoZIhvcNAQEL
BQAwETEPMA0GA1UEAwwGdGVzdGNhMB4XDTI2MDkwODA3NTczN1oXDTI3MDkwODA3
NTczN1owETEPMA0GA1UEAwwGdGVzdGNhMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A
MIIBCgKCAQEAysJqbjbazjOtfFGwyju4iL3GPpV7EiLHC+ylqssjtEOeOmH7x/nL
lakUw8d5wNerMtsha6L6CXvrloKbNgGMmZ6XNrvXcgHzxAq8Lm62Gm+6jc/M3ASV
uEuMLOAdwvuBrzZpeMwrpBSgBSETkKIcrLGiUbCW14wRjkM9xdtSKrzAwlvp0AIh
mm0N2H+AI8wi23VBmqP/dlh6PxqIUxNftPqputee+dOPDyoRL41TrYhruTCTViU5
p9Lw7fkCXeP5p7m5B4b6Fe/HZxqz3DLukXFlRjUtmX/9xwrLZFV15NZDRZ94LIpw
EGzeZcSyqZKNSUnfoQtmFph5br505XiNfwIDAQABo1MwUTAdBgNVHQ4EFgQU9136
wHKbv30UeC7Syauw58dlDRowHwYDVR0jBBgwFoAU9136wHKbv30UeC7Syauw58dl
DRowDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEATSuRRn2bfs23
Ixg9AOjdmrtNciWJ4lWZsbBmxkMSfhbvKgwatbvZUINdeF2MHxP0Ne/Omb9ToHpi
4G3+BSHAB0V4gBIgti/vQI/R5OfW/f5xu1VgEeLZ/f5FTIMlxtcHr1ybbaxg2e8j
UPls5wo0I4FlxCe2qLRWP2xiHSB/UwnjUjbw/UGEd6q2vKaDozuHWQctojRxDi1t
grubJzad+j1FeXEDmJuM7ZN91U5oEbycHH1Nj7RYQGWjT6i2mXIijorvb5WjOKUp
uSe02gSkNpStED1tN4YzA7z1S/tyLw8rQeAQL3CfAz9tMZHcv6cW0nkCMXpNNJc7
xEeZn/ELwg==
-----END CERTIFICATE-----`)

	// WithInsecure applied after WithCACert should override
	fetcher := NewOCIFetcher(
		WithCACert(caCert),
		WithInsecure(),
	)

	transport, ok := fetcher.transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.TLSClientConfig)
	// InsecureSkipVerify should be true since it was applied last
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
}

// Helper types and functions

type authEntry struct {
	Username string
	Password string
	Auth     string
}

func createDockerConfig(t *testing.T, auths map[string]authEntry) []byte {
	t.Helper()

	cfg := struct {
		Auths map[string]dockerAuthConfig `json:"auths"`
	}{
		Auths: make(map[string]dockerAuthConfig),
	}

	for registry, entry := range auths {
		cfg.Auths[registry] = dockerAuthConfig{
			Username: entry.Username,
			Password: entry.Password,
			Auth:     entry.Auth,
		}
	}

	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	return data
}

// TestPluginAnnotation verifies the constant
func TestPluginAnnotation(t *testing.T) {
	assert.Equal(t, "io.backstage.dynamic-packages", PluginAnnotation)
}

// TestTransportIsConfigurable verifies transport configuration
func TestTransportIsConfigurable(t *testing.T) {
	customTransport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
		},
	}

	// Verify default transport works
	fetcher := NewOCIFetcher()
	assert.Equal(t, http.DefaultTransport, fetcher.transport)

	// Verify WithInsecure creates a new transport
	fetcher2 := NewOCIFetcher(WithInsecure())
	assert.NotEqual(t, customTransport, fetcher2.transport)
}

func TestMovePluginContent(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, srcDir string)
		expectError bool
		errorMsg    string
	}{
		{
			name: "plugin in subdirectory",
			setup: func(t *testing.T, srcDir string) {
				pluginDir := filepath.Join(srcDir, "my-plugin")
				require.NoError(t, os.MkdirAll(pluginDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "package.json"), []byte(`{"name":"my-plugin"}`), 0644))
				require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "index.js"), []byte(`module.exports = {}`), 0644))
			},
			expectError: false,
		},
		{
			name: "plugin at root level",
			setup: func(t *testing.T, srcDir string) {
				require.NoError(t, os.WriteFile(filepath.Join(srcDir, "package.json"), []byte(`{"name":"root-plugin"}`), 0644))
				require.NoError(t, os.WriteFile(filepath.Join(srcDir, "index.js"), []byte(`module.exports = {}`), 0644))
			},
			expectError: false,
		},
		{
			name: "no package.json anywhere",
			setup: func(t *testing.T, srcDir string) {
				subDir := filepath.Join(srcDir, "some-dir")
				require.NoError(t, os.MkdirAll(subDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(subDir, "readme.txt"), []byte("no plugin here"), 0644))
			},
			expectError: true,
			errorMsg:    "no plugin content found",
		},
		{
			name: "multiple subdirs picks first with package.json",
			setup: func(t *testing.T, srcDir string) {
				// Create two subdirs, only second has package.json
				dir1 := filepath.Join(srcDir, "aaa-no-plugin")
				dir2 := filepath.Join(srcDir, "bbb-plugin")
				require.NoError(t, os.MkdirAll(dir1, 0755))
				require.NoError(t, os.MkdirAll(dir2, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(dir1, "readme.txt"), []byte("not a plugin"), 0644))
				require.NoError(t, os.WriteFile(filepath.Join(dir2, "package.json"), []byte(`{"name":"bbb-plugin"}`), 0644))
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srcDir := t.TempDir()
			destDir := filepath.Join(t.TempDir(), "dest")

			tt.setup(t, srcDir)

			err := movePluginContent(srcDir, destDir)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				// Verify package.json exists at destination
				_, err := os.Stat(filepath.Join(destDir, "package.json"))
				assert.NoError(t, err, "package.json should exist in destination")
			}
		})
	}
}
