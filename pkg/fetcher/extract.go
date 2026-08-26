package fetcher

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultMaxEntrySize is the default maximum file size for extraction (40MB)
const DefaultMaxEntrySize = 40_000_000

// getMaxEntrySize returns the maximum entry size from MAX_ENTRY_SIZE env var or default
func getMaxEntrySize() int64 {
	if val := os.Getenv("MAX_ENTRY_SIZE"); val != "" {
		if size, err := strconv.ParseInt(val, 10, 64); err == nil && size > 0 {
			return size
		}
	}
	return DefaultMaxEntrySize
}

// extractTarGz extracts a gzipped tarball from a reader to destDir
func extractTarGz(r io.Reader, destDir string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() { _ = gzr.Close() }()

	return extractTar(gzr, destDir)
}

// extractTarGzBytes extracts a gzipped tarball from bytes to destDir
func extractTarGzBytes(data []byte, destDir string) error {
	return extractTarGz(bytes.NewReader(data), destDir)
}

// extractTarBytes extracts an uncompressed tarball from bytes to destDir
func extractTarBytes(data []byte, destDir string) error {
	return extractTar(bytes.NewReader(data), destDir)
}

// extractTar extracts a tarball from a reader to destDir
func extractTar(r io.Reader, destDir string) error {
	tr := tar.NewReader(r)
	maxEntrySize := getMaxEntrySize()

	// Ensure destDir exists and get absolute path for validation
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}
	destDir, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Check header size before extraction (fail fast)
		if header.Size > maxEntrySize {
			return fmt.Errorf("entry %q exceeds maximum size: %d > %d", header.Name, header.Size, maxEntrySize)
		}

		// Construct and validate safe path
		target, err := safePath(destDir, header.Name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := extractFile(tr, target, header.Size, maxEntrySize); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// Skip symlinks - not needed for catalog extraction and avoids path traversal risks
			continue
		}
	}
	return nil
}

// safePath constructs a safe path within destDir, preventing path traversal attacks.
// Returns error if the resulting path would escape destDir.
func safePath(destDir, name string) (string, error) {
	// Clean the name to remove any . or .. components
	cleaned := filepath.Clean(name)
	if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("invalid tar path: %s", name)
	}

	// Join with destination and verify it's still within destDir
	target := filepath.Join(destDir, cleaned)
	if !strings.HasPrefix(target, destDir+string(os.PathSeparator)) && target != destDir {
		return "", fmt.Errorf("invalid tar path: %s", name)
	}

	return target, nil
}

// extractFile extracts a single file from tar reader.
// expectedSize is the size declared in the tar header.
// maxSize is the maximum allowed size (defense in depth).
func extractFile(tr *tar.Reader, path string, expectedSize, maxSize int64) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	// Use expected size from header, but cap at maxSize (defense in depth)
	limit := expectedSize
	if limit > maxSize {
		limit = maxSize
	}
	// Add 1 byte to detect if actual content exceeds declared size
	written, err := io.CopyN(f, tr, limit+1)
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}
	// If we read limit+1 bytes, the file is larger than declared
	if written > limit {
		return fmt.Errorf("file %q actual size exceeds declared size", path)
	}
	return nil
}
