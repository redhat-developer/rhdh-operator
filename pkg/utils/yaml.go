package utils

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func ReadYamlFilesFromDir(dir string) ([]*unstructured.Unstructured, error) {

	if !DirectoryExists(dir) {
		return []*unstructured.Unstructured{}, nil
	}

	var objects []*unstructured.Unstructured
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !IsYamlFile(d.Name()) {
			return nil
		}

		objs, err := ReadYamlFile(path)
		if err != nil {
			return fmt.Errorf("failed to read YAML file %s: %w", path, err)
		}
		objects = append(objects, objs...)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return objects, nil
}

func ReadYamlFile(path string) ([]*unstructured.Unstructured, error) {
	fpath := filepath.Clean(path)
	if _, err := os.Stat(fpath); err != nil {
		return nil, err
	}
	conf, err := os.ReadFile(fpath)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML file: %w", err)
	}

	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(conf), 1000)
	var objects []*unstructured.Unstructured
	for {
		obj := &unstructured.Unstructured{}
		err := dec.Decode(obj)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to decode YAML: %w", err)
		}
		objects = append(objects, obj)
	}

	return objects, nil
}

func ReadYamlContent(content string) ([]*unstructured.Unstructured, error) {
	// Create a YAML decoder from the content
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(content)), 1000)
	var objects []*unstructured.Unstructured

	// Decode the content into unstructured objects
	for {
		obj := &unstructured.Unstructured{}
		err := dec.Decode(obj)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to decode YAML content: %w", err)
		}
		objects = append(objects, obj)
	}

	return objects, nil
}

func IsYamlFile(filename string) bool {
	ext := filepath.Ext(filename)
	return ext == ".yaml" || ext == ".yml"
}

func DirectoryExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}
