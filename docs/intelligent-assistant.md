## Intelligent Assistant Installation and Configuration

### Overview

Red Hat Developer Hub Intelligent Assistant provides AI-powered developer assistance through an integrated chat interface, offering contextual help with code, documentation, and workflow guidance. The Intelligent Assistant flavour bundles all required plugins and configurations to enable this AI-assisted experience.

### What's Included

The Intelligent Assistant flavour (as of v2.1) consists of the following dynamic plugins:

**Developer Hub Intelligent Assistant:**
- `@red-hat-developer-hub/backstage-plugin-intelligent-assistant` - Frontend UI with chat interface, floating action button, and drawer components
- `@red-hat-developer-hub/backstage-plugin-intelligent-assistant-backend` - Backend services for AI processing

These plugins talk to **Lightspeed Core**, which the flavour deploys as a sidecar (`lightspeed-core`).

### Prerequisites

To use Intelligent Assistant, you need:
- Red Hat Developer Hub 2.1 or later
- Access to any LLM of your choosing (configured in a Lightspeed Core `lightspeed-stack.yaml` file)

### Enabling Intelligent Assistant

#### Using the Flavour (Recommended)

Starting from version 2.1, RHDH includes an Intelligent Assistant flavour. It is **enabled by default**. The requisite containers are running but inert until a Secret is updated with sufficient metadata to interact with an LLM for which you have access.

A Backstage CR with an empty spec includes Intelligent Assistant:

```yaml
apiVersion: rhdh.redhat.com/v1alpha5
kind: Backstage
metadata:
  name: my-backstage
spec: {}
```

To disable Intelligent Assistant:

```yaml
spec:
  flavours:
    - name: intelligent-assistant
      enabled: false
```

Or disable all default flavours:

```yaml
spec:
  flavours: []
```

> **Note:** The flavour identifier changed from `lightspeed` to `intelligent-assistant`. Existing Backstage CRs that list `name: lightspeed` must be updated.

#### Manual Plugin Configuration

If you prefer to configure plugins manually without using the flavour, refer to the dynamic plugins ConfigMap:

```yaml
includes:
  - dynamic-plugins.default.yaml
plugins:
  - package: 'ref://red-hat-developer-hub-backstage-plugin-intelligent-assistant'
    enabled: true
  - package: 'ref://red-hat-developer-hub-backstage-plugin-intelligent-assistant-backend'
    enabled: true
```

The plugins use the `intelligent-assistant:` app-config namespace (not `lightspeed:`) and the UI route `/intelligent-assistant`. For more information about configuring dynamic plugins, please refer to the [Configuration documentation](configuration.md).

### Configuration

#### LLM Providers

The flavour includes a default Lightspeed Core stack config in `lightspeed-stack.yaml`. The Operator manages the bundled copy, so do not edit it directly. Create a user-managed ConfigMap with your own copy of this file when you need to configure an LLM provider.

To add a provider:

1. Copy the first ConfigMap document from [`configmap-files.yaml`](../config/profile/rhdh/default-config/flavours/intelligent-assistant/configmap-files.yaml) into a new manifest file. Keep the `lightspeed-stack.yaml` data and omit the `rhdh-profile` ConfigMap document. The [`examples/intelligent-assistant.yaml`](../examples/intelligent-assistant.yaml) file also contains a complete user-managed ConfigMap that enables OpenAI.
2. Change `metadata.name` from `lightspeed-stack-config` to a name you own, such as `my-lightspeed-stack`.
3. Under `inference.providers`, keep the `sentence_transformers` entry and uncomment the provider entry you want to use. Set its `id` to a unique value and update any provider-specific fields. For example:

   ```yaml
   inference:
     providers:
       - type: sentence_transformers
       - type: openai
         id: openai
         api_key_env: OPENAI_API_KEY
   ```

   The bundled file includes commented examples for `openai`, `vllm`, and `vertexai`. You can enable more than one provider, but every provider must have a unique `id`.
4. Add the environment variables named by `api_key_env` and any other provider settings to the Secret that you inject into `lightspeed-core`. For the example above, add `OPENAI_API_KEY` to `intelligent-assistant-secrets` and replace the placeholder with your key.
5. Apply the ConfigMap and Secret in the same namespace as the Backstage custom resource.
6. Reference the ConfigMap from the Backstage CR. The `key` must be `lightspeed-stack.yaml`, and the ConfigMap must be mounted into the `lightspeed-core` container at `/app-root` so that it replaces the bundled file:

```yaml
spec:
  application:
    extraFiles:
      configMaps:
        - name: my-lightspeed-stack
          key: lightspeed-stack.yaml
          mountPath: /app-root
          containers:
            - lightspeed-core
```

The default config ships three inference providers commented out: `openai`, `vllm`, and `vertexai`. Set the matching environment variables for each provider you enable. Setting Secret keys alone does not turn a provider on. The [`examples/intelligent-assistant.yaml`](../examples/intelligent-assistant.yaml) example includes a user-managed `my-lightspeed-stack` ConfigMap and lists the supported Secret keys.

`KV_STORE_PATH`, `SQL_STORE_PATH`, `SQLITE_STORE_DIR`, and `OTEL_SDK_DISABLED` are set on the Lightspeed Core sidecar by default. Override them with `spec.application.extraEnvs.envs` (and `containers: [lightspeed-core]`) if you need different values.

#### Question validation

Question validation is opt-in. Set `ENABLE_VALIDATION` to `question_validity` on the Lightspeed Core sidecar (typically via the Intelligent Assistant Secret). Leave it unset to keep the shield disabled. You must also set `VALIDATION_PROVIDER` and `VALIDATION_MODEL_NAME` so they match a provider you uncommented under `inference.providers`.

#### Backend Authentication

The Intelligent Assistant backend requires authentication configuration to connect to your LLM. Configure this through environment variables or app-config:

```yaml
spec:
  application:
    extraEnvs:
      secrets:
        - name: intelligent-assistant-secrets
          containers:
            - lightspeed-core
```

Ensure the secret contains the necessary authentication keys for AI service access. The secret is injected into the Lightspeed Core sidecar.

#### UI Customization

The Intelligent Assistant chat interface appears as:
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
1. Click the Intelligent Assistant floating button to open the chat interface
2. Ask questions about code, documentation, or workflows

### Notes

- The flavour includes all necessary UI components and backend services, including the Lightspeed Core sidecar

For more information about the Flavour-based configuration system, see the [Configuration documentation](configuration.md#flavours).

### Syncing Upstream Lightspeed Core Configs
> [!NOTE]
> This syncing functionality is intended for use by maintainers of the Intelligent Assistant flavour for RHDH.

The Intelligent Assistant flavour vendors configuration files from the upstream [`redhat-developer/rhdh-intelligent-assistant-configs`](https://github.com/redhat-developer/rhdh-intelligent-assistant-configs) repository. A sync script is provided to fetch the latest versions of these files and update the operator tree in place.

#### What Gets Synced

The script fetches three files from the upstream repository and writes them into two local targets:

| Upstream path | Local target | Content |
|---|---|---|
| `lightspeed-core-configs/lightspeed-stack.yaml` | `config/profile/rhdh/default-config/flavours/intelligent-assistant/configmap-files.yaml` | Lightspeed Core stack configuration |
| `lightspeed-core-configs/rhdh-profile.py` | (same ConfigMap file, different YAML document) | RHDH prompt profile |
| `env/default-values.env` | `examples/intelligent-assistant.yaml` | Secret key scaffolding |

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
