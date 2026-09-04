package fetcher

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// PluginAnnotation is the annotation key that identifies valid plugin artifacts
const PluginAnnotation = "io.backstage.dynamic-packages"

// OCIFetcher fetches artifacts from OCI registries
type OCIFetcher struct {
	transport  http.RoundTripper
	keychain   authn.Keychain
	pluginMode bool // if true, validate io.backstage.dynamic-packages annotation
}

// NewOCIFetcher creates a new OCI fetcher
func NewOCIFetcher(opts ...OCIOption) *OCIFetcher {
	c := &OCIFetcher{
		transport: http.DefaultTransport,
		keychain:  authn.DefaultKeychain,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ----------------------------------------------------------------------------
// Options
// ----------------------------------------------------------------------------

// OCIOption configures the OCIFetcher
type OCIOption func(*OCIFetcher)

// WithInsecure disables TLS certificate verification
func WithInsecure() OCIOption {
	return func(c *OCIFetcher) {
		c.transport = &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
	}
}

// WithCACert adds a custom CA certificate
func WithCACert(caCert []byte) OCIOption {
	return func(c *OCIFetcher) {
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(caCert)
		c.transport = &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{RootCAs: pool},
		}
	}
}

// WithDockerConfig sets authentication from dockerconfigjson
func WithDockerConfig(dockerConfig []byte) OCIOption {
	return func(c *OCIFetcher) {
		kc, err := newDockerConfigKeychain(dockerConfig)
		if err == nil {
			c.keychain = kc
		}
	}
}

// WithPluginMode enables plugin artifact handling.
// When enabled:
// - Validates the io.backstage.dynamic-packages annotation exists
// - Extracts the plugin subdirectory (not raw layer content)
// Use this for plugin artifacts; skip for catalog index images.
func WithPluginMode() OCIOption {
	return func(c *OCIFetcher) {
		c.pluginMode = true
	}
}

// ----------------------------------------------------------------------------
// Fetch methods
// ----------------------------------------------------------------------------

// Fetch downloads an OCI artifact and extracts it to destDir.
// Streams directly to extraction without buffering the entire layer in memory.
// For plugin artifacts, extracts the plugin subdirectory content to destDir.
func (c *OCIFetcher) Fetch(ctx context.Context, ref string, destDir string) error {
	// 1. Parse reference
	imgRef, err := name.ParseReference(ref)
	if err != nil {
		return fmt.Errorf("invalid OCI reference %q: %w", ref, err)
	}

	// 2. Configure remote options
	remoteOpts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(c.keychain),
	}
	if c.transport != nil {
		remoteOpts = append(remoteOpts, remote.WithTransport(c.transport))
	}

	// 3. Fetch artifact
	desc, err := remote.Get(imgRef, remoteOpts...)
	if err != nil {
		return fmt.Errorf("failed to fetch %q: %w", ref, err)
	}

	// 4. Get image
	img, err := desc.Image()
	if err != nil {
		return fmt.Errorf("failed to get image: %w", err)
	}

	// 5. Validate plugin artifact (optional)
	if c.pluginMode {
		if err := pluginModeArtifact(img); err != nil {
			return fmt.Errorf("validation failed for %q: %w", ref, err)
		}
	}

	// 6. Get first layer
	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("failed to get layers: %w", err)
	}
	if len(layers) == 0 {
		return fmt.Errorf("artifact has no layers")
	}

	// 7. Stream layer directly to extraction (no buffering)
	reader, err := layers[0].Uncompressed()
	if err != nil {
		return fmt.Errorf("failed to uncompress layer: %w", err)
	}
	defer func() { _ = reader.Close() }()

	// 8. For non-plugin artifacts (e.g., catalog index), extract directly
	if !c.pluginMode {
		return extractTar(reader, destDir)
	}

	// 9. For plugin artifacts, extract to temp first then move plugin subdirectory
	// Use same filesystem as destDir for atomic rename
	tmpDir, err := os.MkdirTemp(filepath.Dir(destDir), ".oci-extract-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := extractTar(reader, tmpDir); err != nil {
		return err
	}

	// 10. Find and move the plugin subdirectory
	// Plugin artifacts have content in a subdirectory named after the plugin
	return movePluginContent(tmpDir, destDir)
}

// ----------------------------------------------------------------------------
// Internal helpers
// ----------------------------------------------------------------------------

// pluginModeArtifact checks that the image has the required plugin annotation.
// Returns nil if valid, error if the annotation is missing.
func pluginModeArtifact(img v1.Image) error {
	manifest, err := img.Manifest()
	if err != nil {
		return fmt.Errorf("failed to get manifest: %w", err)
	}

	// Check manifest annotations
	if manifest.Annotations != nil {
		if _, ok := manifest.Annotations[PluginAnnotation]; ok {
			return nil
		}
	}

	// Check config labels (some builders put annotations there)
	configFile, err := img.ConfigFile()
	if err == nil && configFile != nil && configFile.Config.Labels != nil {
		if _, ok := configFile.Config.Labels[PluginAnnotation]; ok {
			return nil
		}
	}

	return fmt.Errorf("not a valid plugin artifact (missing %s annotation)", PluginAnnotation)
}

// dockerConfigKeychain implements authn.Keychain using dockerconfigjson
type dockerConfigKeychain struct {
	auths map[string]dockerAuthConfig
}

type dockerAuthConfig struct {
	Auth     string `json:"auth"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type dockerConfig struct {
	Auths map[string]dockerAuthConfig `json:"auths"`
}

func newDockerConfigKeychain(data []byte) (*dockerConfigKeychain, error) {
	var cfg dockerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &dockerConfigKeychain{auths: cfg.Auths}, nil
}

func (k *dockerConfigKeychain) Resolve(target authn.Resource) (authn.Authenticator, error) {
	registry := target.RegistryStr()

	// Try exact match first
	if auth, ok := k.auths[registry]; ok {
		return authToAuthenticator(auth)
	}

	// Try with https:// prefix
	if auth, ok := k.auths["https://"+registry]; ok {
		return authToAuthenticator(auth)
	}

	// Try with /v1/ suffix (Docker Hub style)
	if auth, ok := k.auths["https://"+registry+"/v1/"]; ok {
		return authToAuthenticator(auth)
	}

	return authn.Anonymous, nil
}

func authToAuthenticator(auth dockerAuthConfig) (authn.Authenticator, error) {
	if auth.Username != "" && auth.Password != "" {
		return authn.FromConfig(authn.AuthConfig{
			Username: auth.Username,
			Password: auth.Password,
		}), nil
	}

	if auth.Auth != "" {
		decoded, err := base64.StdEncoding.DecodeString(auth.Auth)
		if err != nil {
			return nil, err
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) == 2 {
			return authn.FromConfig(authn.AuthConfig{
				Username: parts[0],
				Password: parts[1],
			}), nil
		}
	}

	return authn.Anonymous, nil
}

// movePluginContent finds the plugin subdirectory in srcDir and moves its contents to destDir.
// Plugin OCI artifacts contain files in a subdirectory named after the plugin.
func movePluginContent(srcDir, destDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("failed to read extracted content: %w", err)
	}

	// Find the first directory that looks like plugin content (has package.json)
	var pluginDir string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(srcDir, entry.Name())
		if _, err := os.Stat(filepath.Join(candidate, "package.json")); err == nil {
			pluginDir = candidate
			break
		}
	}

	// If no directory with package.json found, check if package.json is at root
	if pluginDir == "" {
		if _, err := os.Stat(filepath.Join(srcDir, "package.json")); err == nil {
			// Content is already at root level, just rename
			return os.Rename(srcDir, destDir)
		}
		return fmt.Errorf("no plugin content found (no package.json in any subdirectory)")
	}

	// Move the plugin directory to destination
	return os.Rename(pluginDir, destDir)
}
