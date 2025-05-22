package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func GetPluginDeps(bsName, bsNamespace string, plugins DynamicPlugins) ([]*unstructured.Unstructured, error) {
	dir := filepath.Join(os.Getenv("LOCALBIN"), "plugin-deps")
	pdeps, err := plugins.Dependencies()
	if err != nil {
		return nil, fmt.Errorf("failed to get plugin dependencies: %w", err)
	}

	//get refs from enabled
	var refs []string
	for _, dep := range pdeps {
		if dep.Ref != "" {
			refs = append(refs, dep.Ref)
		}
	}

	return ReadPluginDeps(dir, bsName, bsNamespace, refs)

}

// ReadPluginDeps reads the plugin dependencies from the specified directory
// and returns a slice of unstructured.Unstructured objects.
func ReadPluginDeps(rootDir, bsName, bsNamespace string, enabled []string) ([]*unstructured.Unstructured, error) {

	if !utils.DirectoryExists(rootDir) {
		return []*unstructured.Unstructured{}, nil
	}

	var objects []*unstructured.Unstructured

	// Read the directory tree
	files, err := getDepsFiles(rootDir, enabled)

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

func getDepsFiles(root string, enabledPrefixes []string) ([]string, error) {
	var files []string

	// Read the directory contents
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", root, err)
	}

	// Iterate over the entries and filter by prefixes
	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip directories
		}

		// Check if the file name starts with any of the enabled prefixes
		for _, prefix := range enabledPrefixes {
			if strings.HasPrefix(entry.Name(), prefix) {
				files = append(files, filepath.Join(root, entry.Name()))
				break
			}
		}

		// TODO add some warning if file is not in the enabled list
	}

	return files, nil
}
