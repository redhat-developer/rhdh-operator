package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/redhat-developer/rhdh-operator/pkg/fetcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests for plugin-fetch
// Run with: make dp-installer-test
//
// Environment variables:
//   CATALOG_INDEX_IMAGE - OCI image to test (default: quay.io/rhdh-community/plugin-catalog-index:latest)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestIntegration_NPMFetchWithVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	f := fetcher.NewNPMFetcher()
	destDir := t.TempDir()

	err := f.Fetch(ctx, "is-odd@3.0.1", destDir)
	require.NoError(t, err)

	packageJSON := filepath.Join(destDir, "package.json")
	assert.FileExists(t, packageJSON)

	data, err := os.ReadFile(packageJSON)
	require.NoError(t, err)
	assert.Contains(t, string(data), "is-odd")
	assert.Contains(t, string(data), "3.0.1")
}

func TestIntegration_NPMFetchLatest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	f := fetcher.NewNPMFetcher()
	destDir := t.TempDir()

	err := f.Fetch(ctx, "is-even", destDir)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(destDir, "package.json"))
}

func TestIntegration_NPMFetchScopedPackage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	f := fetcher.NewNPMFetcher()
	destDir := t.TempDir()

	err := f.Fetch(ctx, "@backstage/types@1.2.0", destDir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(destDir, "package.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "@backstage/types")
}

func TestIntegration_NPMFetchWithIntegrity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	f := fetcher.NewNPMFetcher()
	destDir := t.TempDir()

	// Known integrity for is-odd@3.0.1 (npm view is-odd@3.0.1 dist.integrity)
	integrity := "sha512-CQpnWPrDwmP1+SMHXZhtLtJv90yiyVfluGsX5iNCVkrhQtU3TQHsUWPG9wkdk9Lgd5yNpAg9jQEo90CBaXgWMA=="

	err := f.FetchWithIntegrity(ctx, "is-odd@3.0.1", destDir, integrity)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(destDir, "package.json"))
}

func TestIntegration_NPMFetchIntegrityMismatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	f := fetcher.NewNPMFetcher()
	destDir := t.TempDir()

	wrongIntegrity := "sha512-wronghashAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="

	err := f.FetchWithIntegrity(ctx, "is-odd@3.0.1", destDir, wrongIntegrity)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "integrity")
}

func TestIntegration_OCIFetchPublicImage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	image := getEnv("CATALOG_INDEX_IMAGE", "quay.io/rhdh-community/plugin-catalog-index:latest")

	f := fetcher.NewOCIFetcher()
	destDir := t.TempDir()

	err := f.Fetch(ctx, image, destDir)
	require.NoError(t, err)

	entries, err := os.ReadDir(destDir)
	require.NoError(t, err)
	assert.NotEmpty(t, entries)
	assert.DirExists(t, filepath.Join(destDir, "catalog-entities"))
}

func TestIntegration_FetcherRouting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	f := fetcher.New()

	t.Run("npm package", func(t *testing.T) {
		destDir := t.TempDir()
		err := f.Fetch(ctx, "@backstage/types@1.2.0", destDir)
		require.NoError(t, err)
		assert.FileExists(t, filepath.Join(destDir, "package.json"))
	})

	t.Run("oci image", func(t *testing.T) {
		image := getEnv("CATALOG_INDEX_IMAGE", "quay.io/rhdh-community/plugin-catalog-index:latest")
		destDir := t.TempDir()
		err := f.Fetch(ctx, "oci://"+image, destDir)
		require.NoError(t, err)
		assert.DirExists(t, filepath.Join(destDir, "catalog-entities"))
	})
}

func TestAtomicExtraction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	outputDir := t.TempDir()
	tempDir := filepath.Join(outputDir, "plugin.tmp")
	destDir := filepath.Join(outputDir, "plugin")

	// Failed extraction should leave no directory when cleaned up
	f := fetcher.NewNPMFetcher()
	wrongIntegrity := "sha512-wronghashAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="
	err := f.FetchWithIntegrity(ctx, "is-odd@3.0.1", tempDir, wrongIntegrity)
	assert.Error(t, err)
	_ = os.RemoveAll(tempDir) // cleanup as downloadPackages does

	entries, _ := os.ReadDir(outputDir)
	assert.Empty(t, entries, "failed extraction should leave no directories")

	// Successful extraction with atomic rename
	err = f.Fetch(ctx, "is-odd@3.0.1", tempDir)
	require.NoError(t, err)
	require.NoError(t, os.Rename(tempDir, destDir))

	assert.DirExists(t, destDir)
	assert.NoDirExists(t, tempDir)
	assert.FileExists(t, filepath.Join(destDir, "package.json"))
}

func TestPluginName(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"oci://quay.io/rhdh/plugin-foo:latest", "plugin-foo"},
		{"oci://quay.io/rhdh/plugin-foo@sha256:abc123", "plugin-foo"},
		{"https://example.com/plugin.tar.gz", "plugin"},
		{"@backstage/plugin-catalog@1.0.0", "plugin-catalog"},
		{"@npm:@backstage/plugin-foo@1.0.0", "plugin-foo"},
		{"lodash@4.17.21", "lodash"},
		{"file:/local/path/plugin.tgz", "plugin"},
		// Registry with port - should not confuse port for tag
		{"oci://registry.example.com:5000/org/plugin-bar:v1.0.0", "plugin-bar"},
		{"oci://registry.example.com:5000/org/plugin-bar", "plugin-bar"},
		{"oci://localhost:5000/plugin-test:latest", "plugin-test"},
		{"oci://localhost:5000/plugin-test", "plugin-test"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := pluginName(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}
