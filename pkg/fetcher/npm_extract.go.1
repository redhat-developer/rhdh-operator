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
)

// extractTarGzWithStripPrefix extracts a gzipped tarball, stripping a path prefix from entries
// This is useful for npm tarballs which have "package/" prefix
func extractTarGzWithStripPrefix(data []byte, destDir, stripPrefix string) error {
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	return extractTarWithStripPrefix(gzr, destDir, stripPrefix)
}

// extractTarWithStripPrefix extracts a tarball, stripping a path prefix from entries
func extractTarWithStripPrefix(r io.Reader, destDir, stripPrefix string) error {
	tr := tar.NewReader(r)

	// Ensure destDir exists
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
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

		target := filepath.Join(destDir, name)

		// Security: prevent path traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid tar path: %s", header.Name)
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
			if err := extractFile(tr, target, header.Mode); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// Handle symlinks - validate they don't escape destDir
			linkTarget := filepath.Join(filepath.Dir(target), header.Linkname)
			if !strings.HasPrefix(filepath.Clean(linkTarget), filepath.Clean(destDir)) {
				return fmt.Errorf("symlink escapes destination: %s -> %s", header.Name, header.Linkname)
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		}
	}
	return nil
}
