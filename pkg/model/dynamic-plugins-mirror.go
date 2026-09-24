package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

// ImageDigestMirrors defines mirror configuration (compatible with OpenShift IDMS)
type ImageDigestMirrors struct {
	ImageDigestMirrors []ImageDigestMirror `yaml:"imageDigestMirrors,omitempty" json:"imageDigestMirrors,omitempty"`
}

// ImageDigestMirror represents a single mirror configuration
type ImageDigestMirror struct {
	Source  string   `yaml:"source" json:"source"`
	Mirrors []string `yaml:"mirrors" json:"mirrors"`
}

// ApplyMirror transforms an OCI image reference using mirror config
// Only applies to oci:// URLs - npm packages, HTTP URLs, and file paths are unchanged
// Implements IDMS "most specific namespace match" rule: when multiple sources match,
// the longest/most specific source wins (e.g., quay.io/foo/bar beats quay.io/foo)
func ApplyMirror(ref string, mirrors *ImageDigestMirrors) string {
	if mirrors == nil || len(mirrors.ImageDigestMirrors) == 0 {
		return ref // Early return for performance when no mirrors configured
	}

	// Only mirror OCI images
	if !strings.HasPrefix(ref, "oci://") {
		return ref
	}

	// Remove oci:// prefix for matching
	ociRef := strings.TrimPrefix(ref, "oci://")

	// Find the most specific (longest) matching source
	var bestMatch *ImageDigestMirror
	var bestMatchLen int

	for i := range mirrors.ImageDigestMirrors {
		m := &mirrors.ImageDigestMirrors[i]
		if isOCIMatch(ociRef, m.Source) {
			// Most specific match = longest source string
			if len(m.Source) > bestMatchLen {
				bestMatch = m
				bestMatchLen = len(m.Source)
			}
		}
	}

	// Apply the most specific mirror
	if bestMatch != nil && len(bestMatch.Mirrors) > 0 {
		mirrored := strings.Replace(ociRef, bestMatch.Source, bestMatch.Mirrors[0], 1)
		return "oci://" + mirrored
	}

	return ref
}

// isOCIMatch checks if source matches the beginning of ref at an OCI boundary.
// Valid matches require source to be followed by: /, :, @, or end of string.
// This prevents "quay.io/foo" from matching "quay.io/foobar" and
// "quay.io" from matching "quay.io.example.com".
func isOCIMatch(ref, source string) bool {
	if !strings.HasPrefix(ref, source) {
		return false
	}

	// Exact match
	if len(ref) == len(source) {
		return true
	}

	// Check boundary - must be followed by valid OCI delimiter
	nextChar := ref[len(source)]

	// '/' and '@' are always valid delimiters
	if nextChar == '/' || nextChar == '@' {
		return true
	}

	// ':' is only valid for tags, not registry ports.
	// If source contains '/', it's a repository path and ':' indicates a tag.
	// If source has no '/', it's just a registry and ':' could be a port.
	if nextChar == ':' && strings.Contains(source, "/") {
		return true
	}

	return false
}

// GetMirrorConfig reads mirror configuration from mounted file
// Returns nil if file doesn't exist (no mirroring)
func GetMirrorConfig() (*ImageDigestMirrors, error) {
	// When empty: relative path "plugins-mirror/mirrors.yaml" (for make run with -C bin)
	// When set in cluster: absolute path "/plugins-mirror/mirrors.yaml"
	mirrorFile := filepath.Join(os.Getenv("LOCALBIN"), "plugins-mirror", "mirrors.yaml")
	return readMirrorConfigFromFile(mirrorFile)
}

// readMirrorConfigFromFile reads mirror configuration from specified file path
// Returns nil if file doesn't exist (no mirroring)
func readMirrorConfigFromFile(filePath string) (*ImageDigestMirrors, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No file = no mirroring
		}
		return nil, fmt.Errorf("failed to read mirror config file: %w", err)
	}

	var mirrors ImageDigestMirrors
	if err := yaml.Unmarshal(data, &mirrors); err != nil {
		return nil, fmt.Errorf("failed to parse mirror config: %w", err)
	}

	return &mirrors, nil
}
