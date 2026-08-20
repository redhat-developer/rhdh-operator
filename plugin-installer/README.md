# RHDH Plugin Installer

A bash script for downloading dynamic plugins from various sources (OCI registries, HTTP, NPM) with parallel execution support.

## Overview

This script downloads and extracts dynamic plugins to a specified directory. It supports multiple package sources and handles authentication, integrity verification, and concurrent downloads.

## Container Image

### Pre-built Image

```
quay.io/rhdh-community/plugin-installer:next
```

The image is based on Red Hat UBI 9 Micro with skopeo for OCI registry downloads.

### Building Images

Two Dockerfile variants are provided:

| Dockerfile | OCI Tool | Description                                              |
|------------|----------|----------------------------------------------------------|
| `Dockerfile.skopeo` | skopeo | **Recommended.** Red Hat certified, FIPS compliant.      |
| `Dockerfile.oras` | oras | Lighter experimental image, uses oras for OCI downloads. |

Build using Make targets:

```bash
# Build skopeo variant (default)
make install-dp-build

# Push to registry
make install-dp-push

# Or build with custom image name
make install-dp-build RELATED_IMAGE_plugin_installer=myregistry/my-plugin-installer:v1

# Build oras variant manually
docker build -f plugin-installer/Dockerfile.oras -t myregistry/plugin-installer:oras .
```

## Usage

```bash
./install_plugins.sh [input_file] [output_dir] [parallel_jobs]
```

### Arguments

| Argument | Default | Description |
|----------|---------|-------------|
| `input_file` | `/input/packages.txt` | File containing plugin URLs (one per line) |
| `output_dir` | `/dynamic-plugins-root` | Directory to install plugins |
| `parallel_jobs` | `4` | Number of parallel downloads |

### Environment Variables

| Variable | Description |
|----------|-------------|
| `INPUT_FILE` | Alternative to first argument |
| `OUTPUT_DIR` | Alternative to second argument |
| `PARALLEL_JOBS` | Alternative to third argument |
| `OCI_TOOL` | Force OCI tool: `oras` or `skopeo` (auto-detected if not set) |
| `NPM_REGISTRY` | NPM registry URL (default: `https://registry.npmjs.org`) |
| `NPM_AUTH_TOKEN` | NPM authentication token |
| `NPM_CONFIG_USERCONFIG` | Path to .npmrc file (default: `~/.npmrc`) |
| `SKIP_INTEGRITY_CHECK` | Set to `true` to skip integrity verification |
| `CATALOG_INDEX_IMAGE` | OCI image containing catalog-entities for Extensions UI |
| `CATALOG_ENTITIES_EXTRACT_DIR` | Directory for extracted catalog entities (default: `/tmp/extensions`) |

## Input File Format

One plugin per line. Comments (`#`) and empty lines are ignored.

```
# Format: url [integrity]
# Integrity is optional (sha512-..., sha384-..., or sha256-...)

# OCI registry (integrity ignored - digest in URL provides verification)
oci://ghcr.io/backstage/plugins/catalog@sha256:abc123...

# HTTP with integrity
https://example.com/plugin.tgz sha512-K3mCHKQ9sVh8o2F...

# NPM scoped package
@backstage/plugin-catalog@1.10.0

# NPM scoped package with integrity (overrides registry)
@backstage/plugin-techdocs@1.8.0 sha512-abc123...

# NPM unscoped package
is-odd@3.0.1
```

## Supported URL Formats

### OCI Registry (`oci://`)

Downloads from OCI-compliant registries (GHCR, Quay, Docker Hub, etc.).

```
oci://ghcr.io/org/repo/plugin@sha256:abc123...
oci://quay.io/namespace/plugin:tag
```

**Tools:** Uses `skopeo` (preferred) or `oras` (fallback). At least one must be installed.

### HTTP/HTTPS

Downloads tarballs via HTTP(S).

```
https://example.com/plugin-1.0.0.tgz
https://registry.example.com/packages/plugin.tar.gz
```

**Integrity:** Optional. If provided, verifies SHA hash before extraction.

### NPM Packages

Downloads from NPM registry (or private registry via .npmrc).

```
# Scoped packages (start with @)
@backstage/plugin-catalog
@backstage/plugin-catalog@1.10.0

# Unscoped packages
lodash
is-odd@3.0.1
```

**Features:**
- Reads registry URL and auth token from `.npmrc`
- Supports scoped and unscoped packages
- Resolves `latest` version if not specified
- Verifies integrity (from registry or user-provided)

### Local Directory (`file:` protocol)

Copies from local filesystem directory.

```
# Absolute paths
file:/path/to/plugin
file:///path/to/plugin    # also valid (empty authority)

# Relative paths
file:./path/to/plugin     # explicit relative
file:path/to/plugin       # also relative
```

**Note:** Bare relative paths (`./path`) without `file:` prefix are not supported.

## NPM Configuration

The script reads NPM configuration from `.npmrc`:

```ini
# ~/.npmrc
registry=https://npm.private.com
//npm.private.com/:_authToken=your_token_here
```

Environment variables can override:
```bash
NPM_REGISTRY=https://npm.example.com \
NPM_AUTH_TOKEN=secret_token \
./install_plugins.sh packages.txt ./plugins
```

## Catalog Index (Extensions UI)

When `CATALOG_INDEX_IMAGE` is set, the script extracts catalog entities from the OCI image:

```bash
CATALOG_INDEX_IMAGE=quay.io/rhdh/plugin-catalog-index:1.10 \
CATALOG_ENTITIES_EXTRACT_DIR=/extensions \
./install_plugins.sh packages.txt ./plugins
```

The script looks for `catalog-entities/extensions` in the image layers and copies them to `CATALOG_ENTITIES_EXTRACT_DIR/catalog-entities`.

## Integrity Verification

Packages can be verified using SHA-256, SHA-384, or SHA-512 hashes.

**Format:** `algorithm-base64hash` (e.g., `sha512-K3mCHKQ9sVh8o2F...`)

**Behavior:**
- **HTTP:** Uses provided integrity or skips verification
- **NPM:** Uses provided integrity, falls back to registry's integrity
- **OCI:** Uses digest in URL (integrity field ignored)

**Skip verification:**
```bash
SKIP_INTEGRITY_CHECK=true ./install_plugins.sh
```

## Lock Management

The script uses a lock file to prevent concurrent installations:

- **Lock file:** `<output_dir>/install-dynamic-plugins.lock`
- **Stale lock detection:** Automatically removes locks from dead processes
- **Graceful shutdown:** Releases lock on exit (normal, error, or signal)

## Signal Handling

- **SIGTERM:** Forwarded to child processes for graceful shutdown
- **SIGKILL:** Cannot be trapped; stale lock is cleaned up on next run

## OCI Tools

### skopeo (preferred)

```bash
# Install on macOS
brew install skopeo

# Install on RHEL/Fedora
dnf install skopeo
```

Red Hat certified, FIPS compliant. Uses `skopeo copy` to dir: transport.

### oras (fallback)

```bash
# Install on macOS
brew install oras

# Install on Linux
curl -LO https://github.com/oras-project/oras/releases/download/v1.1.0/oras_1.1.0_linux_amd64.tar.gz
tar -xzf oras_1.1.0_linux_amd64.tar.gz -C /usr/local/bin oras
```

Uses `oras copy --to-oci-layout` for optimized single-operation downloads.

## Examples

### Basic Usage

```bash
# Download plugins from packages.txt to ./plugins
./install_plugins.sh packages.txt ./plugins

# Use 8 parallel downloads
./install_plugins.sh packages.txt ./plugins 8
```

### Environment Variables

```bash
INPUT_FILE=my-plugins.txt \
OUTPUT_DIR=/opt/plugins \
PARALLEL_JOBS=2 \
./install_plugins.sh
```

### Private NPM Registry

```bash
NPM_REGISTRY=https://npm.mycompany.com \
NPM_AUTH_TOKEN=secret123 \
./install_plugins.sh packages.txt ./plugins
```

### Container Usage

```bash
docker run -v $(pwd)/packages.txt:/input/packages.txt \
           -v $(pwd)/plugins:/dynamic-plugins-root \
           quay.io/rhdh-community/plugin-installer:next
```

## Testing

A test suite is provided in `install_plugins_test.sh`.

### Run Unit Tests

```bash
./plugin-installer/install_plugins_test.sh
```

### Run with Integration Tests

Integration tests require network access to download real packages.

```bash
RUN_INTEGRATION_TESTS=true ./plugin-installer/install_plugins_test.sh
```

### Test Coverage

| Category | Tests |
|----------|-------|
| `parse_npmrc()` | 7 tests - .npmrc parsing, registry, auth token |
| `url_encode()` | 3 tests - scoped package URL encoding |
| `verify_integrity()` | 6 tests - SHA verification, skip conditions |
| Integration | 1 test - real npm package download |

## Exit Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 1 | Error (missing input file, download failure, etc.) |

## Troubleshooting

### "Neither oras nor skopeo found"

Install at least one OCI tool:
```bash
brew install oras  # or skopeo
```

### "failed to fetch package metadata"

Check NPM registry accessibility:
```bash
curl -sL https://registry.npmjs.org/is-odd | head -100
```

### "integrity verification failed"

The downloaded file doesn't match the expected hash. Verify the integrity string is correct or set `SKIP_INTEGRITY_CHECK=true`.

### Lock file issues

If the script was killed (SIGKILL), a stale lock may remain. The script auto-detects and removes stale locks, but you can manually remove:
```bash
rm -f /dynamic-plugins-root/install-dynamic-plugins.lock
```
