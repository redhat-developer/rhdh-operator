package fetcher

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// copyLocal copies a local file or directory to destDir
func copyLocal(src string, destDir string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", src, err)
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	if info.IsDir() {
		return copyDir(src, destDir)
	}
	return copyFile(src, filepath.Join(destDir, filepath.Base(src)))
}

// copyDir recursively copies a directory
func copyDir(src, dest string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return err
			}
			if err := copyDir(srcPath, destPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, destPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyFile copies a single file
func copyFile(src, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = destFile.Close() }()

	_, err = io.Copy(destFile, srcFile)
	return err
}
