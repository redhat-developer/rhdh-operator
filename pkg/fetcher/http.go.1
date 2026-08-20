package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	// Create dest directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	// Detect if it's a tarball and extract, or save directly
	contentType := resp.Header.Get("Content-Type")
	if isTarball(url, contentType) {
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

		return extractTarGzBytes(data, destDir)
	}

	// Save as single file
	filename := filepath.Base(url)
	return saveFile(resp.Body, filepath.Join(destDir, filename))
}

// isTarball checks if the URL or content type indicates a tarball
func isTarball(url, contentType string) bool {
	// Check URL extension
	lowerURL := strings.ToLower(url)
	if strings.HasSuffix(lowerURL, ".tar.gz") ||
		strings.HasSuffix(lowerURL, ".tgz") ||
		strings.HasSuffix(lowerURL, ".tar") {
		return true
	}

	// Check content type
	if strings.Contains(contentType, "application/gzip") ||
		strings.Contains(contentType, "application/x-gzip") ||
		strings.Contains(contentType, "application/x-tar") ||
		strings.Contains(contentType, "application/x-compressed-tar") {
		return true
	}

	return false
}

// saveFile saves reader content to a file
func saveFile(r io.Reader, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, r)
	return err
}
