package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// PlatformAnnotation is the annotation key used to specify which platform
	// a plugin dependency resource should be applied to.
	// Value should be "ocp" for OpenShift or "k8s" for Kubernetes.
	// If not specified, the resource is applied on all platforms.
	PlatformAnnotation = "rhdh.redhat.com/platform"
)

func GetPluginDeps(backstage api.Backstage, plugins DynamicPlugins, scheme *runtime.Scheme, platformExt string) ([]*unstructured.Unstructured, error) {

	dir, ok := os.LookupEnv("PLUGIN_DEPS_DIR_backstage")
	if !ok {
		dir = filepath.Join(os.Getenv("LOCALBIN"), "plugin-deps")
	}

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

	objs, err := ReadPluginDeps(dir, backstage.Name, backstage.Namespace, refs, platformExt)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin dependencies: %w", err)
	}

	for _, obj := range objs {
		if obj.GetNamespace() == "" {
			obj.SetNamespace(backstage.Namespace)
		}
		err = controllerutil.SetControllerReference(&backstage, obj, scheme)
		if err != nil {
			return nil, fmt.Errorf("failed to set controller reference for plugin dependency %s: %w", obj.GetName(), err)
		}
	}

	return objs, nil
}

// ReadPluginDeps reads the plugin dependencies from the specified directory
// and returns a slice of unstructured.Unstructured objects.
// Resources with rhdh.redhat.com/platform annotation are filtered by platformExt.
func ReadPluginDeps(rootDir, bsName, bsNamespace string, enabled []string, platformExt string) ([]*unstructured.Unstructured, error) {

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
		// DEPRECATED: {{backstage-name}} and {{backstage-ns}} syntax will be replaced with
		// Go template syntax {{.Backstage.Name}} and {{.Backstage.Namespace}} in a future release.
		// See utils.ApplyTemplate() for the new implementation.
		modifiedContent := strings.ReplaceAll(string(content), "{{backstage-name}}", bsName)
		modifiedContent = strings.ReplaceAll(modifiedContent, "{{backstage-ns}}", bsNamespace)

		// Parse the modified content
		objs, err := utils.ReadYamlContent(modifiedContent)

		if err != nil {
			return nil, fmt.Errorf("failed to read YAML file %s: %w", file, err)
		}

		// Filter by platform annotation
		for _, obj := range objs {
			if matchesPlatform(obj, platformExt) {
				objects = append(objects, obj)
			}
		}
	}

	return objects, nil
}

// matchesPlatform checks if the object should be applied on the given platform.
// Returns true if:
// - platformExt is empty (no filtering)
// - The object has no platform annotation (applies to all platforms)
// - The object's platform annotation matches the given platformExt
func matchesPlatform(obj *unstructured.Unstructured, platformExt string) bool {
	if platformExt == "" {
		return true
	}
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return true
	}
	platform, exists := annotations[PlatformAnnotation]
	if !exists {
		return true
	}
	return platform == platformExt
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

	}

	return files, nil
}
