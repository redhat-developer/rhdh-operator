package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const EnabledPluginsDepsFile = "plugin-dependencies"

// ReadPluginDeps reads the plugin dependencies from the specified directory
// and returns a slice of unstructured.Unstructured objects.
func ReadPluginDeps(rootDir, bsName, bsNamespace string, enabledDirs []string) ([]*unstructured.Unstructured, error) {

	if !utils.DirectoryExists(rootDir) {
		return []*unstructured.Unstructured{}, nil
	}

	var objects []*unstructured.Unstructured

	// Read the directory tree
	files, err := processDepsTree(rootDir, enabledDirs)

	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if !utils.IsYamlFile(file) {
			continue
		}

		// Read file content
		content, err := os.ReadFile(file)
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

func processDepsTree(root string, enabledDirs []string) ([]string, error) {
	// Normalize and store allowed directories
	enabledMap := make(map[string]bool)
	for _, dir := range enabledDirs {
		enabledMap[filepath.Join(root, dir)] = true
	}
	files := []string{}

	// Traverse the directory tree
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {

		if err != nil {
			return err
		}

		// Always allow traversal of directories
		if d.IsDir() {
			fmt.Println("Traversing directory:", path)
			return nil
		}

		// Only process files if the directory (or its parent) is allowed
		if isEnabled(filepath.Dir(path), enabledMap) {
			fmt.Println("Reading file:", path)
			files = append(files, path)
		} else {
			fmt.Println("Skipping file:", path)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func isEnabled(path string, enabledMap map[string]bool) bool {
	// Check if the path or any of its parent directories is in the allowed list
	for {
		if enabledMap[path] {
			return true
		}
		parent := filepath.Dir(path)
		if parent == path || parent == "." || parent == "/" { // Reached the root
			break
		}
		path = parent
	}
	return false
}
