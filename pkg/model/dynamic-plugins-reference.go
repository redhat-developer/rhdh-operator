package model

import (
	"fmt"
	"strings"
)

const inheritSuffix = ":{{inherit}}"
const refPrefix = "ref://"

// resolveReferences resolves all reference types in plugin package URLs.
// Currently supports:
//   - {{inherit}}: inherits version/digest from base plugins
//   - ref://: references another plugin by name (TODO)
func resolveReferences(plugins []DynaPlugin, basePlugins []DynaPlugin) ([]DynaPlugin, error) {
	resolved := make([]DynaPlugin, len(plugins))
	copy(resolved, plugins)

	// Build lookup map for base plugins (used by inherit resolver)
	baseURLMap := buildBaseURLMap(basePlugins)

	for i := range resolved {
		plugin := &resolved[i]
		var err error

		switch {
		case strings.Contains(plugin.Package, inheritSuffix):
			resolved[i].Package, err = resolveInheritReference(plugin.Package, baseURLMap)
		case strings.HasPrefix(plugin.Package, refPrefix):
			resolved[i].Package, err = resolveRefReference(plugin.Package, basePlugins)
		default:
			continue
		}

		if err != nil {
			return nil, err
		}
	}

	return resolved, nil
}

// buildBaseURLMap creates a lookup map from base URL to full package URL.
func buildBaseURLMap(basePlugins []DynaPlugin) map[string]string {
	baseURLMap := make(map[string]string)
	for i := range basePlugins {
		plugin := &basePlugins[i]
		if plugin.Package == "" {
			continue
		}
		baseURL := plugin.BaseURL()
		if baseURL != "" {
			baseURLMap[baseURL] = plugin.Package
		}
	}
	return baseURLMap
}

// resolveInheritReference resolves a single {{inherit}} reference.
// For example, oci://registry/plugin:{{inherit}} will be replaced with
// oci://registry/plugin@sha256:abc123 if found in baseURLMap.
func resolveInheritReference(packageURL string, baseURLMap map[string]string) (string, error) {
	// Parse package to extract !plugin-path suffix if present
	var pluginPath string
	if idx := strings.LastIndex(packageURL, "!"); idx != -1 {
		pluginPath = packageURL[idx:] // includes "!"
		packageURL = packageURL[:idx]
	}

	// Extract base URL (strip :{{inherit}})
	baseURL := strings.Replace(packageURL, inheritSuffix, "", 1)

	// Look up the full URL in basePlugins
	fullURL, found := baseURLMap[baseURL]
	if !found {
		return "", fmt.Errorf("cannot resolve {{inherit}} reference: no matching plugin found for base URL %q in default plugins", baseURL)
	}

	// If user specified !plugin-path, use it; otherwise use full default URL
	if pluginPath != "" {
		// Extract image part from default (without !plugin-path)
		if idx := strings.LastIndex(fullURL, "!"); idx != -1 {
			fullURL = fullURL[:idx]
		}
		return fullURL + pluginPath, nil
	}

	return fullURL, nil
}

// resolveRefReference resolves a ref:// reference by looking up plugin by name.
// For example, ref://my-plugin will be replaced with the full package URL
// of the plugin named "my-plugin" in basePlugins.
func resolveRefReference(packageURL string, basePlugins []DynaPlugin) (string, error) {
	// TODO: implement ref:// resolution
	// ref://<plugin-name> should look up the plugin by name in basePlugins
	return "", fmt.Errorf("ref:// references are not yet implemented: %s", packageURL)
}

// BaseURL extracts the base URL from a plugin package URL
// by removing the tag or digest suffix.
// For example:
//   - oci://registry/plugin:tag -> oci://registry/plugin
//   - oci://registry/plugin@sha256:abc -> oci://registry/plugin
//   - ./local/path -> ./local/path (unchanged)
func (p *DynaPlugin) BaseURL() string {
	packageURL := p.Package

	// Only process OCI URLs
	if !strings.HasPrefix(packageURL, "oci://") {
		return packageURL
	}

	// Strip !plugin-path suffix first if present
	if idx := strings.LastIndex(packageURL, "!"); idx != -1 {
		packageURL = packageURL[:idx]
	}

	// Handle OCI URLs with digest (@sha256:...)
	if idx := strings.LastIndex(packageURL, "@"); idx != -1 {
		return packageURL[:idx]
	}

	// Handle OCI URLs with tag (:tag)
	schemeEnd := len("oci://")
	rest := packageURL[schemeEnd:]
	if idx := strings.LastIndex(rest, ":"); idx != -1 {
		return packageURL[:schemeEnd+idx]
	}

	// No tag or digest found
	return packageURL
}
