## Lightspeed Installation and Configuration

### Overview

Red Hat Developer Hub Lightspeed provides AI-powered developer assistance through an integrated chat interface, offering contextual help with code, documentation, and workflow guidance. The Lightspeed flavour bundles all required plugins and configurations to enable this AI-assisted experience.

### What's Included

The Lightspeed flavour (as of v1.10) consists of the following dynamic plugins:

**Lightspeed Core:**
- `backstage-plugin-lightspeed` - Frontend UI with chat interface, floating action button, and drawer components
- `backstage-plugin-lightspeed-backend` - Backend services for AI processing



### Prerequisites

To use Lightspeed, you need:
- Red Hat Developer Hub 1.10 or later
- Access to any LLM of your choosing (which you set up in the Llama Stack config.yaml configuration)

### Enabling Lightspeed

#### Using the Flavour (Recommended)

Starting from version 1.10, RHDH includes Lightspeed as an **enabled-by-default** flavour. For new deployments, Lightspeed is automatically active (the requisite containers are running) but inert (there is a Secret which must be updated with sufficient metadata to interact with a LLM for which you have access):

```yaml
apiVersion: rhdh.redhat.com/v1alpha6
kind: Backstage
metadata:
  name: my-backstage
spec: {}
```

To explicitly enable Lightspeed:

```yaml
spec:
  flavours:
    - name: lightspeed
      enabled: true
```

To disable Lightspeed:

```yaml
spec:
  flavours:
    - name: lightspeed
      enabled: false
```

Or disable all default flavours:

```yaml
spec:
  flavours: []
```

#### Manual Plugin Configuration

If you prefer to configure plugins manually without using the flavour, refer to the dynamic plugins ConfigMap:

```yaml
includes:
  - dynamic-plugins.default.yaml
plugins:
  - package: oci://ghcr.io/redhat-developer/rhdh-plugin-export-overlays/red-hat-developer-hub-backstage-plugin-lightspeed:bs_1.45.3__1.2.3
    disabled: false
  - package: oci://ghcr.io/redhat-developer/rhdh-plugin-export-overlays/red-hat-developer-hub-backstage-plugin-lightspeed-backend:bs_1.45.3__1.2.3
    disabled: false
```

For more information about configuring dynamic plugins, please refer to the [Configuration documentation](configuration.md).

### Configuration

#### Backend Authentication

The Lightspeed backend requires authentication configuration to connect to your LLM. Configure this through environment variables or app-config:

```yaml
spec:
  application:
    extraEnvs:
      secrets:
        - name: lightspeed-credentials
```

Ensure the secret contains the necessary authentication keys for AI service access.

#### UI Customization

The Lightspeed chat interface appears as:
- A floating action button (FAB) in the bottom-right corner
- A drawer panel that slides in from the right
- Mount points at various locations in the application

These UI elements are configured through the plugin's `pluginConfig` and can be customized in the dynamic plugins configuration.


### Features

**AI Chat Interface:**
- Contextual assistance for code development
- Documentation navigation and search
- Workflow guidance and best practices
- Natural language queries about the software catalog


### Usage

Once enabled, users can:
1. Click the Lightspeed floating button to open the chat interface
2. Ask questions about code, documentation, or workflows

### Notes

- Starting with version 1.10, Lightspeed is enabled by default for all RHDH deployments, including new installs and upgrades of existing instances.
- The flavour includes all necessary UI components and backend services

For more information about the Flavour-based configuration system, see the [Configuration documentation](configuration.md#flavours).

### Syncing Upstream Lightspeed Configs
> [!NOTE]
> This syncing functionality is intended for use by maintainers of the Lightspeed flavour for RHDH.

The Lightspeed flavour vendors configuration files from the upstream [`redhat-ai-dev/lightspeed-configs`](https://github.com/redhat-ai-dev/lightspeed-configs) repository. A sync script is provided to fetch the latest versions of these files and update the operator tree in place.

#### What Gets Synced

The script fetches four files from the upstream repository and writes them into two local targets:

| Upstream path | Local target | Content |
|---|---|---|
| `llama-stack-configs/config.yaml` | `config/profile/rhdh/default-config/flavours/lightspeed/configmap-files.yaml` | Llama Stack server configuration |
| `lightspeed-core-configs/lightspeed-stack.yaml` | (same ConfigMap file, different YAML document) | Lightspeed Core stack configuration |
| `lightspeed-core-configs/rhdh-profile.py` | (same ConfigMap file, different YAML document) | RHDH prompt profile |
| `env/default-values.env` | `examples/lightspeed.yaml` | Secret key scaffolding |

#### Running the Script

Sync from the default upstream branch (`main`):

```bash
./hack/sync-lightspeed-configs.sh
```

#### Syncing from a Release Branch or Tag

Use the `--ref` flag to sync from a specific branch, tag, or commit:

```bash
./hack/sync-lightspeed-configs.sh --ref release-1.10
```

This is useful when preparing a release and the operator needs to pin its vendored configs to a stable upstream ref rather than `main`.

If the upstream content has not changed, the script prints `already up to date` and leaves the files untouched.
