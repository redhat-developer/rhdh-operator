package fetcher

import (
	"context"
	"fmt"
	"strings"
)

// Fetcher downloads plugins/artifacts from various sources
type Fetcher struct {
	oci  *OCIFetcher
	http *HTTPFetcher
	npm  *NPMFetcher
}

// FetcherOption configures the Fetcher
type FetcherOption func(*Fetcher)

// New creates a new Fetcher with the given options
func New(opts ...FetcherOption) *Fetcher {
	f := &Fetcher{
		oci:  NewOCIFetcher(),
		http: NewHTTPFetcher(),
		npm:  NewNPMFetcher(),
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// WithOCIOptions configures the OCI fetcher with the given options
func WithOCIOptions(opts ...OCIOption) FetcherOption {
	return func(f *Fetcher) {
		f.oci = NewOCIFetcher(opts...)
	}
}

// WithNPMOptions configures the NPM fetcher with the given options
func WithNPMOptions(opts ...NPMOption) FetcherOption {
	return func(f *Fetcher) {
		f.npm = NewNPMFetcher(opts...)
	}
}

// Fetch downloads from a URL and extracts to destDir
func (f *Fetcher) Fetch(ctx context.Context, url string, destDir string) error {
	return f.FetchWithIntegrity(ctx, url, destDir, "")
}

// FetchWithIntegrity downloads from a URL with optional integrity verification
func (f *Fetcher) FetchWithIntegrity(ctx context.Context, url string, destDir string, integrity string) error {
	switch {
	case strings.HasPrefix(url, "oci://"):
		// OCI uses digest in URL for verification, integrity param ignored
		return f.oci.Fetch(ctx, strings.TrimPrefix(url, "oci://"), destDir)
	case strings.HasPrefix(url, "https://"), strings.HasPrefix(url, "http://"):
		return f.http.FetchWithIntegrity(ctx, url, destDir, integrity)
	case strings.HasPrefix(url, "@") || strings.Contains(url, "@npm:"):
		return f.npm.FetchWithIntegrity(ctx, url, destDir, integrity)
	case strings.HasPrefix(url, "file:"):
		return copyLocal(strings.TrimPrefix(url, "file:"), destDir)
	default:
		return fmt.Errorf("unsupported URL scheme: %s", url)
	}
}
