package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ReadPluginDeps reads the plugin dependencies from the specified directory
// and returns a slice of unstructured.Unstructured objects.
func ReadPluginDeps(rootDir, bsName, bsNamespace string, enabledDirs []string) ([]*unstructured.Unstructured, error) {

	if !utils.DirectoryExists(rootDir) {
		return []*unstructured.Unstructured{}, nil
	}

	var objects []*unstructured.Unstructured

	// Read the directory tree
	files, err := getDepsFiles(rootDir, enabledDirs)

	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if !utils.IsYamlFile(file) {
			continue
		}

		// Read file content
		content, err := os.ReadFile(filepath.Clean(file))
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", file, err)
		}

		// Perform substitutions
		modifiedContent := strings.ReplaceAll(string(content), "{{backstage-name}}", bsName)
		modifiedContent = strings.ReplaceAll(modifiedContent, "{{backstage-ns}}", bsNamespace)

		// Parse the modified content
		objs, err := utils.ReadYamlContent(modifiedContent)

		if err != nil {
			return nil, fmt.Errorf("failed to read YAML file %s: %w", file, err)
		}
		objects = append(objects, objs...)
	}

	return objects, nil
}

func getDepsFiles(root string, enabledDirs []string) ([]string, error) {
	var files []string

	// Iterate over the specified directories
	for _, dir := range enabledDirs {
		dirPath := filepath.Join(root, dir)

		// Read the directory contents
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory %s: %w", dirPath, err)
		}

		// Collect only files from the first level
		for _, entry := range entries {
			if !entry.IsDir() { // Skip subdirectories
				files = append(files, filepath.Join(dirPath, entry.Name()))
			}
		}
	}

	return files, nil
}
