package model

import (
	"fmt"
	"strings"
)

const inheritSuffix = ":{{inherit}}"
const refPrefix = "ref://"
const ociPrefix = "oci://"
const httpsPrefix = "https://"
const httpPrefix = "http://"
const localPrefix = "./"

// resolveReferences resolves all reference types in plugin package URLs.
//
// Supported package URL formats:
//
// Catalog references (resolved by plugin name lookup):
//   - ref://plugin-name: Returns full package URL from base plugins matching by name.
//     Example: ref://backstage-plugin-catalog → oci://quay.io/rhdh/backstage-plugin-catalog@sha256:abc123
//   - oci://...{{inherit}}: Inherits version/digest from base plugins matching by name.
//     The registry/path in the user URL is ignored - only the plugin name matters.
//     Example: oci://any-registry/backstage-plugin-catalog:{{inherit}} → oci://quay.io/rhdh/backstage-plugin-catalog@sha256:abc123
//     User can override !plugin-path: oci://x/plugin:{{inherit}}!custom-path → uses custom-path instead of base's path
//
// Direct links (no resolution needed):
//   - oci://...: OCI image reference
//   - https://...: HTTPS URL to plugin archive
//   - http://...: HTTP URL to plugin archive
//   - ./path: Local filesystem path
//
// Any other prefix returns an error.
func resolveReferences(plugins []DynaPlugin, basePlugins []DynaPlugin) ([]DynaPlugin, error) {
	resolved := make([]DynaPlugin, len(plugins))
	copy(resolved, plugins)

	for i := range resolved {
		plugin := &resolved[i]
		if plugin.Package == "" {
			continue
		}

		var err error

		switch {
		case strings.HasPrefix(plugin.Package, refPrefix):
			// Catalog search by name
			resolved[i].Package, err = resolveRefReference(plugin.Package, basePlugins)
		case strings.Contains(plugin.Package, inheritSuffix):
			// Catalog search by name, inherit version/digest
			resolved[i].Package, err = resolveInheritReference(plugin.Package, basePlugins)
		case plugin.IsDirectLink():
			// Direct link - no resolution needed
			continue
		default:
			return nil, fmt.Errorf("unsupported package URL format %q: must start with oci://, https://, http://, ./ or use ref:// for catalog lookup", plugin.Package)
		}

		if err != nil {
			return nil, err
		}
	}

	return resolved, nil
}

// IsDirectLink returns true if the package URL is a direct link that doesn't need resolution.
func (p *DynaPlugin) IsDirectLink() bool {
	return strings.HasPrefix(p.Package, ociPrefix) ||
		strings.HasPrefix(p.Package, httpsPrefix) ||
		strings.HasPrefix(p.Package, httpPrefix) ||
		strings.HasPrefix(p.Package, localPrefix)
}

// resolveInheritReference resolves a single {{inherit}} reference by looking up plugin by name.
// The registry and path in the user's URL are ignored - only the plugin name (last path component) matters.
//
// Examples:
//   - oci://any-registry/path/plugin-foo:{{inherit}} matches base plugin oci://quay.io/rhdh/plugin-foo@sha256:abc
//   - oci://x/plugin-foo:{{inherit}}!custom-path uses base's version but user's plugin-path
func resolveInheritReference(packageURL string, basePlugins []DynaPlugin) (string, error) {
	// Parse package to extract !plugin-path suffix if present
	var pluginPath string
	if idx := strings.LastIndex(packageURL, "!"); idx != -1 {
		pluginPath = packageURL[idx:] // includes "!"
		packageURL = packageURL[:idx]
	}

	// Extract plugin name from the package URL (strip :{{inherit}} first)
	tempPackage := strings.Replace(packageURL, inheritSuffix, "", 1)
	tempPlugin := DynaPlugin{Package: tempPackage}
	pluginName := tempPlugin.Name()

	if pluginName == "" {
		return "", fmt.Errorf("cannot resolve {{inherit}} reference: unable to extract plugin name from %q", packageURL)
	}

	// Look up the plugin by name in basePlugins
	for i := range basePlugins {
		plugin := &basePlugins[i]
		if plugin.Package == "" {
			continue
		}
		if plugin.Name() == pluginName {
			fullURL := plugin.Package

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
	}

	return "", fmt.Errorf("cannot resolve {{inherit}} reference: no plugin named %q found in default plugins", pluginName)
}

// resolveRefReference resolves a ref:// reference by looking up plugin by name.
// Returns the full package URL from basePlugins for the plugin with matching name.
//
// Example: ref://backstage-plugin-catalog → oci://quay.io/rhdh/backstage-plugin-catalog@sha256:abc123
func resolveRefReference(packageURL string, basePlugins []DynaPlugin) (string, error) {
	// Extract the plugin name from ref://<plugin-name>
	refName := strings.TrimPrefix(packageURL, refPrefix)
	if refName == "" {
		return "", fmt.Errorf("invalid ref:// reference: empty plugin name in %q", packageURL)
	}

	// Look up the plugin by name in basePlugins
	for i := range basePlugins {
		plugin := &basePlugins[i]
		if plugin.Package == "" {
			continue
		}
		if plugin.Name() == refName {
			return plugin.Package, nil
		}
	}

	return "", fmt.Errorf("cannot resolve ref:// reference: no plugin named %q found in default plugins", refName)
}

// Name extracts the plugin name from the package URL.
// For example:
//   - oci://quay.io/rhdh/backstage-plugin-techdocs@sha256:abc -> backstage-plugin-techdocs
//   - oci://quay.io/rhdh/backstage-plugin-techdocs:1.0.0 -> backstage-plugin-techdocs
//   - https://example.com/path/backstage-plugin-foo-1.0.0.tgz -> backstage-plugin-foo
//   - ./dynamic-plugins/dist/backstage-plugin-techdocs -> backstage-plugin-techdocs
func (p *DynaPlugin) Name() string {
	packageURL := p.Package

	// Strip !plugin-path suffix if present
	if idx := strings.LastIndex(packageURL, "!"); idx != -1 {
		packageURL = packageURL[:idx]
	}

	// Handle OCI URLs
	if strings.HasPrefix(packageURL, ociPrefix) {
		// Remove oci:// prefix
		url := strings.TrimPrefix(packageURL, ociPrefix)

		// Remove digest (@sha256:...)
		if idx := strings.LastIndex(url, "@"); idx != -1 {
			url = url[:idx]
		}

		// Require a path component (must have "/") - registry-only URLs are invalid
		idx := strings.LastIndex(url, "/")
		if idx == -1 {
			return ""
		}

		// Extract the last path component (the image name, possibly with tag)
		imageName := url[idx+1:]
		if imageName == "" {
			return ""
		}

		// Remove tag (:tag) from image name only (not port from registry)
		if idx := strings.LastIndex(imageName, ":"); idx != -1 {
			imageName = imageName[:idx]
		}

		return imageName
	}

	// Handle HTTP(S) URLs
	if strings.HasPrefix(packageURL, httpsPrefix) || strings.HasPrefix(packageURL, httpPrefix) {
		// Remove scheme
		url := strings.TrimPrefix(packageURL, httpsPrefix)
		url = strings.TrimPrefix(url, httpPrefix)

		// Extract the last path component
		if idx := strings.LastIndex(url, "/"); idx != -1 {
			url = url[idx+1:]
		}

		// Strip query string if present
		if idx := strings.Index(url, "?"); idx != -1 {
			url = url[:idx]
		}

		// Strip common archive extensions
		url = strings.TrimSuffix(url, ".tgz")
		url = strings.TrimSuffix(url, ".tar.gz")

		// Strip version suffix (e.g., -1.0.0, -1.2.3-beta)
		url = stripVersionSuffix(url)

		return url
	}

	// Handle local paths (./path/to/plugin-name)
	if strings.HasPrefix(packageURL, localPrefix) {
		if idx := strings.LastIndex(packageURL, "/"); idx != -1 {
			return packageURL[idx+1:]
		}
		return packageURL
	}

	// Unknown protocol - return empty string
	return ""
}

// stripVersionSuffix removes a trailing version suffix from a plugin name.
// For example: backstage-plugin-foo-1.0.0 -> backstage-plugin-foo
func stripVersionSuffix(name string) string {
	// Look for pattern: -<digit> which typically starts a version
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '-' && i+1 < len(name) && name[i+1] >= '0' && name[i+1] <= '9' {
			return name[:i]
		}
	}
	return name
}
