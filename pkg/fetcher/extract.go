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

// DefaultMaxEntrySize is the default maximum file size for extraction (1GB)
const DefaultMaxEntrySize = 1 << 30

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
	defer gzr.Close()

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

		target := filepath.Join(destDir, header.Name)

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

// extractFile extracts a single file from tar reader
func extractFile(tr *tar.Reader, path string, mode int64) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(mode))
	if err != nil {
		return err
	}
	defer f.Close()

	// Limit copy size to prevent decompression bombs
	// Uses MAX_ENTRY_SIZE env var or defaults to 1GB
	maxSize := getMaxEntrySize()
	_, err = io.CopyN(f, tr, maxSize)
	if err == io.EOF {
		return nil
	}
	return err
}
