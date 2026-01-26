package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha5"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	imagev1 "github.com/openshift/api/image/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// ImageStreamsDirEnvVar is the environment variable name for the imagestreams directory
	ImageStreamsDirEnvVar = "IMAGESTREAMS_DIR_backstage"
	// InternalRegistryEnvVar is the environment variable set on the init container
	// to tell it to use the internal OCP registry
	InternalRegistryEnvVar = "OCP_INTERNAL_REGISTRY_URL"
	// DefaultInternalRegistryURL is the default OCP internal registry service URL
	DefaultInternalRegistryURL = "image-registry.openshift-image-registry.svc:5000"
)

// GetImageStreams reads ImageStream manifests from the configured directory
// and returns them for application during reconciliation.
// This is only applicable when running on OpenShift.
func GetImageStreams(backstage bsv1.Backstage, scheme *runtime.Scheme, isOpenShift bool) ([]*unstructured.Unstructured, error) {
	// Only create ImageStreams on OpenShift
	if !isOpenShift {
		return []*unstructured.Unstructured{}, nil
	}

	dir, ok := os.LookupEnv(ImageStreamsDirEnvVar)
	if !ok {
		dir = filepath.Join(os.Getenv("LOCALBIN"), "imagestreams")
	}

	if !utils.DirectoryExists(dir) {
		return []*unstructured.Unstructured{}, nil
	}

	objs, err := readImageStreamManifests(dir, backstage.Name, backstage.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to read imagestream manifests: %w", err)
	}

	// Set controller reference for all ImageStream objects
	for _, obj := range objs {
		if obj.GetNamespace() == "" {
			obj.SetNamespace(backstage.Namespace)
		}
		err = controllerutil.SetControllerReference(&backstage, obj, scheme)
		if err != nil {
			return nil, fmt.Errorf("failed to set controller reference for imagestream %s: %w", obj.GetName(), err)
		}
	}

	return objs, nil
}

// readImageStreamManifests reads all YAML files from the directory and parses them as unstructured objects
func readImageStreamManifests(dir, bsName, bsNamespace string) ([]*unstructured.Unstructured, error) {
	var objects []*unstructured.Unstructured

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read imagestreams directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		if !utils.IsYamlFile(filePath) {
			continue
		}

		content, err := os.ReadFile(filepath.Clean(filePath))
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		// Perform placeholder substitutions
		modifiedContent := strings.ReplaceAll(string(content), "{{backstage-name}}", bsName)
		modifiedContent = strings.ReplaceAll(modifiedContent, "{{backstage-ns}}", bsNamespace)

		objs, err := utils.ReadYamlContent(modifiedContent)
		if err != nil {
			return nil, fmt.Errorf("failed to parse YAML file %s: %w", filePath, err)
		}

		objects = append(objects, objs...)
	}

	return objects, nil
}

// ImageStreamName generates a consistent name for an ImageStream based on the image reference
func ImageStreamName(imageRef string) string {
	// Extract the image name from the full reference
	// e.g., registry.redhat.io/rhdh/rhdh-hub-rhel9:1.9 -> rhdh-hub-rhel9
	parts := strings.Split(imageRef, "/")
	lastPart := parts[len(parts)-1]

	// Remove tag or digest
	if idx := strings.Index(lastPart, ":"); idx != -1 {
		lastPart = lastPart[:idx]
	}
	if idx := strings.Index(lastPart, "@"); idx != -1 {
		lastPart = lastPart[:idx]
	}

	// Sanitize for Kubernetes naming conventions
	return sanitizeK8sName(lastPart)
}

// sanitizeK8sName ensures the name is valid for Kubernetes resources
func sanitizeK8sName(name string) string {
	// Replace any non-alphanumeric characters (except dashes) with dashes
	result := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r + 32 // lowercase
		}
		return '-'
	}, name)

	// Remove leading/trailing dashes
	result = strings.Trim(result, "-")

	// Ensure it doesn't exceed max length
	if len(result) > 63 {
		result = result[:63]
	}

	return result
}

// BuildImageStreamForImage creates an ImageStream manifest for a given OCI image reference
func BuildImageStreamForImage(imageRef, namespace string) *imagev1.ImageStream {
	name := ImageStreamName(imageRef)

	// Parse the image reference to get registry, repository, and tag
	registry, repository, tag := parseImageRef(imageRef)

	is := &imagev1.ImageStream{}
	is.SetName(name)
	is.SetNamespace(namespace)
	is.SetLabels(map[string]string{
		"app.kubernetes.io/managed-by": "rhdh-operator",
		"rhdh.redhat.com/imagestream":  "true",
	})

	is.Spec = imagev1.ImageStreamSpec{
		Tags: []imagev1.TagReference{
			{
				Name: tag,
				From: &corev1.ObjectReference{
					Kind: "DockerImage",
					Name: fmt.Sprintf("%s/%s:%s", registry, repository, tag),
				},
				ImportPolicy: imagev1.TagImportPolicy{
					Scheduled: true, // Automatically import and track updates
				},
				ReferencePolicy: imagev1.TagReferencePolicy{
					Type: imagev1.LocalTagReferencePolicy, // Use local pullthrough
				},
			},
		},
	}

	return is
}

// parseImageRef parses an OCI image reference into registry, repository, and tag
func parseImageRef(imageRef string) (registry, repository, tag string) {
	// Default tag
	tag = "latest"

	// Handle digest format
	if idx := strings.Index(imageRef, "@"); idx != -1 {
		tag = imageRef[idx+1:]
		imageRef = imageRef[:idx]
	} else if idx := strings.LastIndex(imageRef, ":"); idx != -1 {
		// Check if this is a port (contains only digits after colon before next slash)
		afterColon := imageRef[idx+1:]
		if slashIdx := strings.Index(afterColon, "/"); slashIdx != -1 {
			// This is likely a port, not a tag - keep the full imageRef
		} else {
			tag = afterColon
			imageRef = imageRef[:idx]
		}
	}

	// Split registry and repository
	parts := strings.SplitN(imageRef, "/", 2)
	if len(parts) == 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
		registry = parts[0]
		repository = parts[1]
	} else {
		// Default to docker.io
		registry = "docker.io"
		repository = imageRef
	}

	return
}
