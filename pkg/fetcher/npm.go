package fetcher

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"strings"
)

// NPMFetcher downloads plugins from NPM registry (no Node.js required)
type NPMFetcher struct {
	registryURL        string
	authToken          string
	skipIntegrityCheck bool
	client             *http.Client
}

// NPMOption configures the NPM fetcher
type NPMOption func(*NPMFetcher)

// NewNPMFetcher creates a new NPM fetcher
func NewNPMFetcher(opts ...NPMOption) *NPMFetcher {
	f := &NPMFetcher{
		registryURL: "https://registry.npmjs.org",
		client:      http.DefaultClient,
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// WithNPMRegistry sets the NPM registry URL
func WithNPMRegistry(url string) NPMOption {
	return func(f *NPMFetcher) {
		if url != "" {
			f.registryURL = strings.TrimSuffix(url, "/")
		}
	}
}

// WithNPMAuthToken sets the NPM auth token
func WithNPMAuthToken(token string) NPMOption {
	return func(f *NPMFetcher) {
		f.authToken = token
	}
}

// WithNPMConfigFile reads registry and auth from .npmrc file
func WithNPMConfigFile(path string) NPMOption {
	return func(f *NPMFetcher) {
		if path == "" {
			return
		}
		registry, token := parseNPMRC(path)
		if registry != "" {
			f.registryURL = strings.TrimSuffix(registry, "/")
		}
		if token != "" {
			f.authToken = token
		}
	}
}

// WithSkipIntegrityCheck skips integrity verification
func WithSkipIntegrityCheck() NPMOption {
	return func(f *NPMFetcher) {
		f.skipIntegrityCheck = true
	}
}

// parseNPMRC parses .npmrc file for registry and auth token
func parseNPMRC(path string) (registry, authToken string) {
	file, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// registry=https://registry.npmjs.org
		if strings.HasPrefix(line, "registry") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				registry = strings.Trim(strings.TrimSpace(parts[1]), "\"'")
			}
		}

		// //registry.npmjs.org/:_authToken=xxx
		if strings.Contains(line, ":_authToken=") {
			parts := strings.SplitN(line, ":_authToken=", 2)
			if len(parts) == 2 {
				authToken = strings.Trim(strings.TrimSpace(parts[1]), "\"'")
			}
		}
	}
	return registry, authToken
}

// Fetch downloads NPM package and extracts to destDir
// Supports formats: @scope/package@version, package@version
func (f *NPMFetcher) Fetch(ctx context.Context, pkg string, destDir string) error {
	return f.FetchWithIntegrity(ctx, pkg, destDir, "")
}

// FetchWithIntegrity downloads NPM package with optional integrity verification
func (f *NPMFetcher) FetchWithIntegrity(ctx context.Context, pkg string, destDir string, userIntegrity string) error {
	// Parse package name and version
	name, version := parseNPMPackage(pkg)

	// If version is "latest", resolve it first
	if version == "latest" {
		resolvedVersion, err := f.resolveLatestVersion(ctx, name)
		if err != nil {
			return err
		}
		version = resolvedVersion
	}

	// Get version-specific metadata from registry (includes integrity)
	tarballURL, registryIntegrity, err := f.getVersionMetadata(ctx, name, version)
	if err != nil {
		return err
	}

	// Use user-provided integrity if available, otherwise use registry integrity
	integrity := userIntegrity
	if integrity == "" {
		integrity = registryIntegrity
	}

	// Download tarball
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tarballURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	if f.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+f.authToken)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", tarballURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, tarballURL)
	}

	// Read body into memory for integrity check
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read tarball: %w", err)
	}

	// Verify integrity if not skipped
	if !f.skipIntegrityCheck && integrity != "" {
		if err := verifyIntegrity(data, integrity); err != nil {
			return fmt.Errorf("integrity verification failed: %w", err)
		}
	}

	// Extract tarball (npm tarballs have "package/" prefix)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	return extractTarGzWithStripPrefix(data, destDir, "package/")
}

// resolveLatestVersion gets the latest version from the full package document
func (f *NPMFetcher) resolveLatestVersion(ctx context.Context, name string) (string, error) {
	metaURL := fmt.Sprintf("%s/%s", f.registryURL, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metaURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	if f.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+f.authToken)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch NPM metadata for %s: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("NPM registry returned %d for %s", resp.StatusCode, name)
	}

	var meta struct {
		DistTags struct {
			Latest string `json:"latest"`
		} `json:"dist-tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return "", fmt.Errorf("failed to parse NPM metadata: %w", err)
	}

	if meta.DistTags.Latest == "" {
		return "", fmt.Errorf("no latest version found for %s", name)
	}

	return meta.DistTags.Latest, nil
}

// getVersionMetadata gets the tarball URL and integrity for a specific version
func (f *NPMFetcher) getVersionMetadata(ctx context.Context, name, version string) (tarballURL, integrity string, err error) {
	// Use version-specific endpoint to avoid grepping through full package doc
	versionURL := fmt.Sprintf("%s/%s/%s", f.registryURL, name, version)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/json")
	if f.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+f.authToken)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch NPM version metadata for %s@%s: %w", name, version, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("NPM registry returned %d for %s@%s", resp.StatusCode, name, version)
	}

	var versionMeta struct {
		Dist struct {
			Tarball   string `json:"tarball"`
			Integrity string `json:"integrity"`
		} `json:"dist"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&versionMeta); err != nil {
		return "", "", fmt.Errorf("failed to parse NPM version metadata: %w", err)
	}

	if versionMeta.Dist.Tarball == "" {
		return "", "", fmt.Errorf("no tarball URL found for %s@%s", name, version)
	}

	return versionMeta.Dist.Tarball, versionMeta.Dist.Integrity, nil
}

// verifyIntegrity verifies data against an SRI hash (sha256-..., sha384-..., sha512-...)
func verifyIntegrity(data []byte, integrity string) error {
	parts := strings.SplitN(integrity, "-", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid integrity format: %s", integrity)
	}

	algorithm := parts[0]
	expected := parts[1]

	var h hash.Hash
	switch algorithm {
	case "sha256":
		h = sha256.New()
	case "sha384":
		h = sha512.New384()
	case "sha512":
		h = sha512.New()
	default:
		return fmt.Errorf("unsupported integrity algorithm: %s", algorithm)
	}

	h.Write(data)
	computed := base64.StdEncoding.EncodeToString(h.Sum(nil))

	if computed != expected {
		return fmt.Errorf("integrity mismatch: expected %s, got %s", expected, computed)
	}

	return nil
}

// parseNPMPackage parses package name and version from various formats
func parseNPMPackage(pkg string) (name, version string) {
	// Handle @npm: prefix
	pkg = strings.TrimPrefix(pkg, "@npm:")

	// Handle @scope/package@version or package@version
	// Find last @ that separates version (not the scope @)
	if idx := strings.LastIndex(pkg, "@"); idx > 0 {
		return pkg[:idx], pkg[idx+1:]
	}
	return pkg, "latest"
}
