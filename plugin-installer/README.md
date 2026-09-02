# RHDH Plugin Installer

Downloads dynamic plugins from various sources (OCI registries, HTTP, NPM) with integrity verification.

## Overview

A Go-based tool (`plugin-fetch`) that downloads and extracts dynamic plugins to a specified directory. Supports multiple package sources, authentication, and integrity verification.

Source code: `cmd/plugin-fetch/`, `pkg/fetcher/`

## Container Image

### Pre-built Image

```
quay.io/rhdh-community/rhdh-plugin-installer:next
```

The image is based on Red Hat UBI 9 Micro with the `plugin-fetch` binary.

### Building Images

Build using Make targets:

```bash
# Run tests
make dp-installer-test

# Build and push multiplatform image
make dp-installer-buildx

# Or build with custom image name
make dp-installer-buildx INSTALL_DP_IMAGE=myregistry/my-plugin-installer:v1
```

## Usage

```bash
plugin-fetch
```

All configuration is via environment variables.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `INPUT_FILE` | `/input/packages.txt` | File containing plugin URLs (one per line) |
| `OUTPUT_DIR` | `/dynamic-plugins-root` | Directory to install plugins |
| `PARALLEL` | `4` | Number of parallel downloads |
| `NPM_REGISTRY` | `https://registry.npmjs.org` | NPM registry URL |
| `NPM_AUTH_TOKEN` | | NPM authentication token |
| `NPM_CONFIG_USERCONFIG` | `~/.npmrc` | Path to .npmrc file |
| `SKIP_INTEGRITY_CHECK` | `false` | Skip integrity verification |
| `DOCKER_CONFIG` | | Path to docker config.json for OCI registry auth |
| `CA_FILE` | | Path to CA certificate file for TLS |
| `INSECURE` | `false` | Skip TLS verification |
| `VALIDATE_DP_ANNOTATION` | `true` | Require `io.backstage.dynamic-packages` annotation on OCI images |
| `CATALOG_INDEX_IMAGE` | | OCI image containing catalog-entities for Extensions UI |
| `CATALOG_ENTITIES_EXTRACT_DIR` | `/tmp/extensions` | Directory for extracted catalog entities |

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

**Auth:** Set `DOCKER_CONFIG` to path of docker config.json for private registries.

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
plugin-fetch
```

## Catalog Index (Extensions UI)

When `CATALOG_INDEX_IMAGE` is set, catalog entities are extracted from the OCI image:

```bash
CATALOG_INDEX_IMAGE=quay.io/rhdh/plugin-catalog-index:1.10 \
CATALOG_ENTITIES_EXTRACT_DIR=/extensions \
plugin-fetch
```

Looks for `catalog-entities/extensions` in the image layers and copies them to `CATALOG_ENTITIES_EXTRACT_DIR/catalog-entities`.

## Integrity Verification

Packages can be verified using SHA-256, SHA-384, or SHA-512 hashes.

**Format:** `algorithm-base64hash` (e.g., `sha512-K3mCHKQ9sVh8o2F...`)

**Behavior:**
- **HTTP:** Uses provided integrity or skips verification
- **NPM:** Uses provided integrity, falls back to registry's integrity
- **OCI:** Uses digest in URL (integrity field ignored)

**Skip verification:**
```bash
SKIP_INTEGRITY_CHECK=true plugin-fetch
```

## Lock Management

A lock file prevents concurrent installations:

- **Lock file:** `<output_dir>/install-dynamic-plugins.lock`
- **Stale lock detection:** Automatically removes locks from dead processes
- **Graceful shutdown:** Releases lock on exit (normal, error, or signal)

## Signal Handling

- **SIGTERM/SIGINT:** Triggers graceful shutdown, cancels in-flight downloads
- **SIGKILL:** Cannot be trapped; stale lock is cleaned up on next run

## Examples

### Basic Usage

```bash
# Download plugins from packages.txt to ./plugins
INPUT_FILE=packages.txt OUTPUT_DIR=./plugins plugin-fetch

# Use 8 parallel downloads
PARALLEL=8 plugin-fetch
```

### Private NPM Registry

```bash
NPM_REGISTRY=https://npm.mycompany.com \
NPM_AUTH_TOKEN=secret123 \
plugin-fetch
```

### Private OCI Registry

```bash
DOCKER_CONFIG=/path/to/docker/config.json \
plugin-fetch
```

### Container Usage

```bash
docker run -v $(pwd)/packages.txt:/input/packages.txt \
           -v $(pwd)/plugins:/dynamic-plugins-root \
           quay.io/rhdh-community/rhdh-plugin-installer:next
```

## Testing

Tests are provided in Go for the fetcher package and plugin-fetch command.

### Run Tests

```bash
make dp-installer-test
```

This runs:
- `pkg/fetcher/` unit tests (NPM parsing, OCI options, extraction, integrity)
- `cmd/plugin-fetch/` integration tests (real NPM/OCI downloads)

### Test Coverage

| Package | Tests |
|---------|-------|
| `pkg/fetcher/npm_test.go` | NPM package parsing, .npmrc parsing, integrity verification |
| `pkg/fetcher/oci_test.go` | OCI options, docker config, keychain resolution |
| `pkg/fetcher/http_test.go` | HTTP fetcher, tarball detection |
| `pkg/fetcher/local_test.go` | Local file/directory copying, file: protocol |
| `pkg/fetcher/extract_test.go` | Tar extraction, path traversal protection, size limits |
| `cmd/plugin-fetch/integration_test.go` | Real NPM/OCI downloads, routing |

## Exit Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 1 | Error (missing input file, download failure, etc.) |

## Troubleshooting

### "failed to fetch package metadata"

Check NPM registry accessibility:
```bash
curl -sL https://registry.npmjs.org/is-odd | head -100
```

### "integrity verification failed"

The downloaded file doesn't match the expected hash. Verify the integrity string is correct or set `SKIP_INTEGRITY_CHECK=true`.

### "x509: certificate signed by unknown authority"

For private registries with custom CA, set `CA_FILE` to the CA certificate path, or set `INSECURE=true` (not recommended for production).

### Lock file issues

If the process was killed (SIGKILL), a stale lock may remain. Stale locks are auto-detected and removed, but you can manually remove:
```bash
rm -f /dynamic-plugins-root/install-dynamic-plugins.lock
```
