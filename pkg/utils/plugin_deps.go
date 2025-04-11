package utils

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func ReadPluginDeps(rootDir string) ([]*unstructured.Unstructured, error) {

	if !DirectoryExists(rootDir) {
		return []*unstructured.Unstructured{}, nil
	}

	var objects []*unstructured.Unstructured

	// Read allowed directories from the "enabled" file
	enabledDirs, err := readEnabledDirs(filepath.Join(rootDir, "enabled"))
	if err != nil {
		return nil, err
	}

	// Read the directory tree
	files, err := processDepsTree(rootDir, enabledDirs)

	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if !isYamlFile(file) {
			continue
		}
		objs, err := ReadYamlFile(file)
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
		enabledMap[filepath.Clean(dir)] = true
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

func readEnabledDirs(filePath string) ([]string, error) {
	var enabledDirs []string

	root := filepath.Dir(filePath)
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Ignore file not found error and return an empty list
			return enabledDirs, nil
		}
		return nil, err
	}
	defer file.Close()

	// Read the file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			//fmt.Println("Adding enabled line:", line)
			//fmt.Println("Adding enabled parent:", parent)
			path := filepath.Join(root, line)
			fmt.Println("Adding enabled directory:", path)
			enabledDirs = append(enabledDirs, path)
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return enabledDirs, nil
}
