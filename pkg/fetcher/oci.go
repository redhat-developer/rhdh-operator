package fetcher

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	transport      http.RoundTripper
	keychain       authn.Keychain
	validatePlugin bool // if true, validate io.backstage.dynamic-packages annotation
}

// OCIOption configures the OCIFetcher
type OCIOption func(*OCIFetcher)

// WithInsecure disables TLS certificate verification
func WithInsecure() OCIOption {
	return func(c *OCIFetcher) {
		c.transport = &http.Transport{
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

// WithPluginValidation enables plugin artifact validation.
// When enabled, Fetch will verify the io.backstage.dynamic-packages annotation exists.
// Use this for plugin artifacts; skip for catalog index images.
func WithPluginValidation() OCIOption {
	return func(c *OCIFetcher) {
		c.validatePlugin = true
	}
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

// FetchResult contains the result of fetching an artifact
type FetchResult struct {
	// Digest is the artifact digest (sha256:...)
	Digest string

	// Content is the extracted artifact content
	Content []byte

	// MediaType of the artifact
	MediaType string
}

// Fetch downloads an OCI artifact and extracts it to destDir
func (c *OCIFetcher) Fetch(ctx context.Context, ref string, destDir string) error {
	result, err := c.FetchContent(ctx, ref)
	if err != nil {
		return err
	}

	// Extract tarball content to destDir (already uncompressed by Uncompressed())
	return extractTarBytes(result.Content, destDir)
}

// FetchContent downloads an OCI artifact and returns its content
func (c *OCIFetcher) FetchContent(ctx context.Context, ref string) (*FetchResult, error) {
	// 1. Parse reference
	imgRef, err := name.ParseReference(ref)
	if err != nil {
		return nil, fmt.Errorf("invalid OCI reference %q: %w", ref, err)
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
		return nil, fmt.Errorf("failed to fetch %q: %w", ref, err)
	}

	// 4. Handle image vs artifact
	img, err := desc.Image()
	if err != nil {
		return nil, fmt.Errorf("failed to get image: %w", err)
	}

	// 5. Validate plugin artifact (optional, for plugins only)
	if c.validatePlugin {
		if err := validatePluginArtifact(img); err != nil {
			return nil, fmt.Errorf("validation failed for %q: %w", ref, err)
		}
	}

	// 6. Extract content from first layer
	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("failed to get layers: %w", err)
	}
	if len(layers) == 0 {
		return nil, fmt.Errorf("artifact has no layers")
	}

	reader, err := layers[0].Uncompressed()
	if err != nil {
		return nil, fmt.Errorf("failed to uncompress layer: %w", err)
	}
	defer func() { _ = reader.Close() }()

	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read layer: %w", err)
	}

	digest, err := img.Digest()
	if err != nil {
		return nil, fmt.Errorf("failed to get digest: %w", err)
	}

	return &FetchResult{
		Digest:  digest.String(),
		Content: content,
	}, nil
}

// validatePluginArtifact checks that the image has the required plugin annotation.
// Returns nil if valid, error if the annotation is missing.
func validatePluginArtifact(img v1.Image) error {
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
