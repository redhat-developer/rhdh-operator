package fetcher

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
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

func TestNewHTTPFetcher(t *testing.T) {
	f := NewHTTPFetcher()
	assert.NotNil(t, f)
	assert.NotNil(t, f.client)
}

func TestHTTPFetcher_FetchTarGz(t *testing.T) {
	// Create a test tarball
	tarball := createTestTarGz(t, map[string]string{
		"package.json": `{"name": "test-plugin"}`,
		"index.js":     "module.exports = {};",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(tarball)
	}))
	defer server.Close()

	f := NewHTTPFetcher()
	destDir := t.TempDir()

	err := f.Fetch(context.Background(), server.URL+"/plugin.tar.gz", destDir)
	require.NoError(t, err)

	// Verify files were extracted
	data, err := os.ReadFile(filepath.Join(destDir, "package.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "test-plugin")
}

func TestHTTPFetcher_RejectsNonTarball(t *testing.T) {
	content := []byte("console.log('hello');")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write(content)
	}))
	defer server.Close()

	f := NewHTTPFetcher()
	destDir := t.TempDir()

	err := f.Fetch(context.Background(), server.URL+"/script.js", destDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a tarball")
}

func TestHTTPFetcher_404Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	f := NewHTTPFetcher()
	destDir := t.TempDir()

	err := f.Fetch(context.Background(), server.URL+"/nonexistent.tar.gz", destDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestHTTPFetcher_WithIntegrity(t *testing.T) {
	tarball := createTestTarGz(t, map[string]string{"test.txt": "test content"})
	integrity := "sha512-" + computeSHA512Base64(tarball)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(tarball)
	}))
	defer server.Close()

	f := NewHTTPFetcher()
	destDir := t.TempDir()

	// Should succeed with correct integrity
	err := f.FetchWithIntegrity(context.Background(), server.URL+"/file.tar.gz", destDir, integrity)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(destDir, "test.txt"))
}

func TestHTTPFetcher_IntegrityMismatch(t *testing.T) {
	content := []byte("actual content")
	wrongIntegrity := "sha512-wronghashAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(createTestTarGz(t, map[string]string{"test.txt": string(content)}))
	}))
	defer server.Close()

	f := NewHTTPFetcher()
	destDir := t.TempDir()

	err := f.FetchWithIntegrity(context.Background(), server.URL+"/file.tar.gz", destDir, wrongIntegrity)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "integrity")
}

func TestHTTPFetcher_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wait for context cancellation
		<-r.Context().Done()
	}))
	defer server.Close()

	f := NewHTTPFetcher()
	destDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := f.Fetch(ctx, server.URL+"/slow.tar.gz", destDir)
	assert.Error(t, err)
}

func TestIsTarball(t *testing.T) {
	tests := []struct {
		url         string
		contentType string
		expected    bool
	}{
		// URL-based detection
		{"/plugin.tar.gz", "", true},
		{"/plugin.tgz", "", true},
		{"/plugin.tar", "", true},
		{"/plugin.js", "", false},
		{"/plugin.json", "", false},

		// Content-type based detection
		{"/plugin", "application/gzip", true},
		{"/plugin", "application/x-gzip", true},
		{"/plugin", "application/x-tar", true},
		{"/plugin", "application/x-compressed-tar", true},
		{"/plugin", "application/json", false},
		{"/plugin", "text/plain", false},

		// Case insensitive URL
		{"/plugin.TAR.GZ", "", true},
		{"/plugin.TGZ", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.url+":"+tt.contentType, func(t *testing.T) {
			result := isTarball(tt.url, tt.contentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsGzipped(t *testing.T) {
	tests := []struct {
		url         string
		contentType string
		expected    bool
	}{
		// Gzipped
		{"/plugin.tar.gz", "", true},
		{"/plugin.tgz", "", true},
		{"/plugin", "application/gzip", true},
		{"/plugin", "application/x-gzip", true},
		{"/plugin", "application/x-compressed-tar", true},

		// Not gzipped (uncompressed tar or other)
		{"/plugin.tar", "", false},
		{"/plugin", "application/x-tar", false},
		{"/plugin.js", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.url+":"+tt.contentType, func(t *testing.T) {
			result := isGzipped(tt.url, tt.contentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHTTPFetcher_FetchUncompressedTar(t *testing.T) {
	// Create an uncompressed tar archive
	tarball := createTestTar(t, map[string]string{
		"package.json": `{"name": "uncompressed-plugin"}`,
		"index.js":     "module.exports = {};",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-tar")
		_, _ = w.Write(tarball)
	}))
	defer server.Close()

	f := NewHTTPFetcher()
	destDir := t.TempDir()

	err := f.Fetch(context.Background(), server.URL+"/plugin.tar", destDir)
	require.NoError(t, err)

	// Verify files were extracted
	data, err := os.ReadFile(filepath.Join(destDir, "package.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "uncompressed-plugin")
}

// ============================================================================
// Helper functions
// ============================================================================

func createTestTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		err := tw.WriteHeader(hdr)
		require.NoError(t, err)
		_, err = tw.Write([]byte(content))
		require.NoError(t, err)
	}

	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())

	return buf.Bytes()
}

func createTestTar(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		err := tw.WriteHeader(hdr)
		require.NoError(t, err)
		_, err = tw.Write([]byte(content))
		require.NoError(t, err)
	}

	require.NoError(t, tw.Close())
	return buf.Bytes()
}

func computeSHA512Base64(data []byte) string {
	hash := sha512.Sum512(data)
	return base64.StdEncoding.EncodeToString(hash[:])
}
