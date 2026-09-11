package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// HTTPFetcher downloads plugins from HTTP/HTTPS URLs
type HTTPFetcher struct {
	client *http.Client
}

// NewHTTPFetcher creates a new HTTP fetcher
func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{client: http.DefaultClient}
}

// Fetch downloads from HTTP URL and extracts to destDir
func (f *HTTPFetcher) Fetch(ctx context.Context, url string, destDir string) error {
	return f.FetchWithIntegrity(ctx, url, destDir, "")
}

// FetchWithIntegrity downloads from HTTP URL with optional integrity verification
func (f *HTTPFetcher) FetchWithIntegrity(ctx context.Context, url string, destDir string, integrity string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	// Create dest directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	// Verify it's a tarball - plugins must be tarballs
	contentType := resp.Header.Get("Content-Type")
	if !isTarball(url, contentType) {
		return fmt.Errorf("HTTP response is not a tarball (Content-Type: %s, URL: %s)", contentType, url)
	}

	// Read body into memory for integrity check
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Verify integrity if provided
	if integrity != "" {
		if err := verifyIntegrity(data, integrity); err != nil {
			return fmt.Errorf("integrity verification failed: %w", err)
		}
	}

	// Extract based on compression type
	if isGzipped(url, contentType) {
		return extractTarGzBytes(data, destDir)
	}
	return extractTarBytes(data, destDir)
}

// isTarball checks if the URL or content type indicates a tarball
func isTarball(url, contentType string) bool {
	return isGzipped(url, contentType) || isUncompressedTar(url, contentType)
}

// isGzipped checks if the URL or content type indicates gzip-compressed content
func isGzipped(url, contentType string) bool {
	lowerURL := strings.ToLower(url)
	if strings.HasSuffix(lowerURL, ".tar.gz") || strings.HasSuffix(lowerURL, ".tgz") {
		return true
	}
	if strings.Contains(contentType, "application/gzip") ||
		strings.Contains(contentType, "application/x-gzip") ||
		strings.Contains(contentType, "application/x-compressed-tar") {
		return true
	}
	return false
}

// isUncompressedTar checks if the URL or content type indicates an uncompressed tar
func isUncompressedTar(url, contentType string) bool {
	if strings.HasSuffix(strings.ToLower(url), ".tar") {
		return true
	}
	if strings.Contains(contentType, "application/x-tar") {
		return true
	}
	return false
}
