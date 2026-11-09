package fetcher

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	securejoin "github.com/cyphar/filepath-securejoin"
)

// extractTarGzWithStripPrefix extracts a gzipped tarball, stripping a path prefix from entries
// This is useful for npm tarballs which have "package/" prefix
func extractTarGzWithStripPrefix(data []byte, destDir, stripPrefix string) error {
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() { _ = gzr.Close() }()

	return extractTarWithStripPrefix(gzr, destDir, stripPrefix)
}

// extractTarWithStripPrefix extracts a tarball, stripping a path prefix from entries
func extractTarWithStripPrefix(r io.Reader, destDir, stripPrefix string) error {
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

		// Strip prefix from name
		name := header.Name
		if stripPrefix != "" && strings.HasPrefix(name, stripPrefix) {
			name = strings.TrimPrefix(name, stripPrefix)
		}
		if name == "" || name == "." {
			continue
		}

		// Check header size before extraction (fail fast)
		if header.Size > maxEntrySize {
			return fmt.Errorf("entry %q exceeds maximum size: %d > %d", header.Name, header.Size, maxEntrySize)
		}

		// Construct safe path using securejoin to prevent path traversal attacks
		target, err := securejoin.SecureJoin(destDir, name)
		if err != nil {
			return fmt.Errorf("invalid tar path %q: %w", header.Name, err)
		}
		// Defense in depth: verify resolved path is within destDir
		if !strings.HasPrefix(target, destDir+string(os.PathSeparator)) && target != destDir {
			return fmt.Errorf("path traversal blocked: %q resolves outside destination", header.Name)
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
			// Skip symlinks - not needed for npm packages and avoids path traversal risks
			continue
		}
	}
	return nil
}
