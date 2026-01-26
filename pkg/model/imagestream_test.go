package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImageStreamName(t *testing.T) {
	tests := []struct {
		name     string
		imageRef string
		expected string
	}{
		{
			name:     "simple image with tag",
			imageRef: "registry.redhat.io/rhdh/rhdh-hub-rhel9:1.9",
			expected: "rhdh-hub-rhel9",
		},
		{
			name:     "image with digest",
			imageRef: "registry.redhat.io/rhdh/rhdh-hub-rhel9@sha256:abc123",
			expected: "rhdh-hub-rhel9",
		},
		{
			name:     "image without tag",
			imageRef: "registry.redhat.io/rhdh/rhdh-hub-rhel9",
			expected: "rhdh-hub-rhel9",
		},
		{
			name:     "image with uppercase letters",
			imageRef: "registry.redhat.io/rhdh/RHDH-Hub-RHEL9:1.9",
			expected: "rhdh-hub-rhel9",
		},
		{
			name:     "plugin catalog index",
			imageRef: "registry.redhat.io/rhdh/rhdh-plugin-index-catalog:1.9",
			expected: "rhdh-plugin-index-catalog",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ImageStreamName(tt.imageRef)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseImageRef(t *testing.T) {
	tests := []struct {
		name       string
		imageRef   string
		wantReg    string
		wantRepo   string
		wantTag    string
	}{
		{
			name:     "full image reference with tag",
			imageRef: "registry.redhat.io/rhdh/rhdh-hub-rhel9:1.9",
			wantReg:  "registry.redhat.io",
			wantRepo: "rhdh/rhdh-hub-rhel9",
			wantTag:  "1.9",
		},
		{
			name:     "image reference with digest",
			imageRef: "registry.redhat.io/rhdh/rhdh-hub-rhel9@sha256:abc123def456",
			wantReg:  "registry.redhat.io",
			wantRepo: "rhdh/rhdh-hub-rhel9",
			wantTag:  "sha256:abc123def456",
		},
		{
			name:     "image without tag defaults to latest",
			imageRef: "registry.redhat.io/rhdh/rhdh-hub-rhel9",
			wantReg:  "registry.redhat.io",
			wantRepo: "rhdh/rhdh-hub-rhel9",
			wantTag:  "latest",
		},
		{
			name:     "quay.io image",
			imageRef: "quay.io/rhdh/plugin-catalog-index:1.9",
			wantReg:  "quay.io",
			wantRepo: "rhdh/plugin-catalog-index",
			wantTag:  "1.9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg, repo, tag := parseImageRef(tt.imageRef)
			assert.Equal(t, tt.wantReg, reg, "registry mismatch")
			assert.Equal(t, tt.wantRepo, repo, "repository mismatch")
			assert.Equal(t, tt.wantTag, tag, "tag mismatch")
		})
	}
}

func TestSanitizeK8sName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "already valid",
			input:    "my-image-name",
			expected: "my-image-name",
		},
		{
			name:     "uppercase to lowercase",
			input:    "My-Image-Name",
			expected: "my-image-name",
		},
		{
			name:     "underscores to dashes",
			input:    "my_image_name",
			expected: "my-image-name",
		},
		{
			name:     "trim leading/trailing dashes",
			input:    "-my-image-name-",
			expected: "my-image-name",
		},
		{
			name:     "long name truncated",
			input:    "this-is-a-very-long-name-that-exceeds-the-kubernetes-resource-name-limit-of-63-characters",
			expected: "this-is-a-very-long-name-that-exceeds-the-kubernetes-resource-n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeK8sName(tt.input)
			assert.Equal(t, tt.expected, result)
			assert.LessOrEqual(t, len(result), 63, "name should not exceed 63 characters")
		})
	}
}
