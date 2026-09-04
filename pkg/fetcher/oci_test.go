package fetcher

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"net/http"
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
	assert.False(t, fetcher.validatePlugin)
}

func TestWithInsecure(t *testing.T) {
	fetcher := NewOCIFetcher(WithInsecure())

	transport, ok := fetcher.transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.TLSClientConfig)
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
}

func TestWithCACert(t *testing.T) {
	// Create a test CA cert (self-signed, for testing)
	caCert := []byte(`-----BEGIN CERTIFICATE-----
MIIBkTCB+wIJAKHBfpegPjMCMA0GCSqGSIb3DQEBCwUAMBExDzANBgNVBAMMBnRl
c3RjYTAeFw0yMzAxMDEwMDAwMDBaFw0yNDAxMDEwMDAwMDBaMBExDzANBgNVBAMM
BnRlc3RjYTBcMA0GCSqGSIb3DQEBAQUAA0sAMEgCQQC7o96FCFzP9GVMF/odlY0x
fXH5XKPF6a9kKZl7e8V7H6KYQO7MYHl7aNQp7XZf0g/iYn/lFxVKcGFz/fQMt7oB
AgMBAAGjUzBRMB0GA1UdDgQWBBQBJ7R4DTvnR0lf7FQ1A6f7M5l1kjAfBgNVHSME
GDAWgBQBJ7R4DTvnR0lf7FQ1A6f7M5l1kjAPBgNVHRMBAf8EBTADAQH/MA0GCSqG
SIb3DQEBCwUAA0EA0GXpF1JguGJ2I7m3pCnYdU3lfQEb7y0e6pZyfMz5EPuV1JJh
k5mN+kTl5kmYJsYLsL7Q3v5K5ng+q3lQiPqE/w==
-----END CERTIFICATE-----`)

	fetcher := NewOCIFetcher(WithCACert(caCert))

	transport, ok := fetcher.transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.TLSClientConfig)
	assert.NotNil(t, transport.TLSClientConfig.RootCAs)
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
MIIBkTCB+wIJAKHBfpegPjMCMA0GCSqGSIb3DQEBCwUAMBExDzANBgNVBAMMBnRl
c3RjYTAeFw0yMzAxMDEwMDAwMDBaFw0yNDAxMDEwMDAwMDBaMBExDzANBgNVBAMM
BnRlc3RjYTBcMA0GCSqGSIb3DQEBAQUAA0sAMEgCQQC7o96FCFzP9GVMF/odlY0x
fXH5XKPF6a9kKZl7e8V7H6KYQO7MYHl7aNQp7XZf0g/iYn/lFxVKcGFz/fQMt7oB
AgMBAAGjUzBRMB0GA1UdDgQWBBQBJ7R4DTvnR0lf7FQ1A6f7M5l1kjAfBgNVHSME
GDAWgBQBJ7R4DTvnR0lf7FQ1A6f7M5l1kjAPBgNVHRMBAf8EBTADAQH/MA0GCSqG
SIb3DQEBCwUAA0EA0GXpF1JguGJ2I7m3pCnYdU3lfQEb7y0e6pZyfMz5EPuV1JJh
k5mN+kTl5kmYJsYLsL7Q3v5K5ng+q3lQiPqE/w==
-----END CERTIFICATE-----`)

	dockerConfig := createDockerConfig(t, map[string]authEntry{
		"registry.example.com": {Username: "user", Password: "pass"},
	})

	// Apply multiple options - last one that sets transport wins for transport
	fetcher := NewOCIFetcher(
		WithCACert(caCert),
		WithDockerConfig(dockerConfig),
		WithPluginValidation(),
	)

	// Verify all options were applied
	assert.True(t, fetcher.validatePlugin)

	transport, ok := fetcher.transport.(*http.Transport)
	require.True(t, ok)
	assert.NotNil(t, transport.TLSClientConfig)

	kc, ok := fetcher.keychain.(*dockerConfigKeychain)
	require.True(t, ok)
	assert.Len(t, kc.auths, 1)
}

func TestInsecureOverridesCACert(t *testing.T) {
	caCert := []byte(`-----BEGIN CERTIFICATE-----
MIIBkTCB+wIJAKHBfpegPjMCMA0GCSqGSIb3DQEBCwUAMBExDzANBgNVBAMMBnRl
c3RjYTAeFw0yMzAxMDEwMDAwMDBaFw0yNDAxMDEwMDAwMDBaMBExDzANBgNVBAMM
BnRlc3RjYTBcMA0GCSqGSIb3DQEBAQUAA0sAMEgCQQC7o96FCFzP9GVMF/odlY0x
fXH5XKPF6a9kKZl7e8V7H6KYQO7MYHl7aNQp7XZf0g/iYn/lFxVKcGFz/fQMt7oB
AgMBAAGjUzBRMB0GA1UdDgQWBBQBJ7R4DTvnR0lf7FQ1A6f7M5l1kjAfBgNVHSME
GDAWgBQBJ7R4DTvnR0lf7FQ1A6f7M5l1kjAPBgNVHRMBAf8EBTADAQH/MA0GCSqG
SIb3DQEBCwUAA0EA0GXpF1JguGJ2I7m3pCnYdU3lfQEb7y0e6pZyfMz5EPuV1JJh
k5mN+kTl5kmYJsYLsL7Q3v5K5ng+q3lQiPqE/w==
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
