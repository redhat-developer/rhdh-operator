## Lightspeed Installation and Configuration

### Overview

Red Hat Developer Hub Lightspeed provides AI-powered developer assistance through an integrated chat interface, offering contextual help with code, documentation, and workflow guidance. The Lightspeed flavour bundles all required plugins and configurations to enable this AI-assisted experience.

### What's Included

The Lightspeed flavour (as of v1.10) consists of the following dynamic plugins:

**Lightspeed Core:**
- `backstage-plugin-lightspeed` - Frontend UI with chat interface, floating action button, and drawer components
- `backstage-plugin-lightspeed-backend` - Backend services for AI processing

**Model Context Protocol (MCP) Tools:**
- `backstage-plugin-mcp-actions-backend` - Backend for MCP actions
- `backstage-plugin-software-catalog-mcp-tool` - AI tools for software catalog queries
- `backstage-plugin-techdocs-mcp-tool` - AI tools for TechDocs navigation and search

### Prerequisites

To use Lightspeed, you need:
- Red Hat Developer Hub 1.10 or later
- Access to any LLM of your choosing (which you set up in the Llama Stack run.yaml configuration)
- (Optional) Custom MCP server configurations for extended capabilities

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
  - package: oci://ghcr.io/redhat-developer/rhdh-plugin-export-overlays/backstage-plugin-mcp-actions-backend:bs_1.45.3__0.1.5
    disabled: false
  - package: oci://ghcr.io/redhat-developer/rhdh-plugin-export-overlays/red-hat-developer-hub-backstage-plugin-software-catalog-mcp-tool:bs_1.45.3__0.4.1
    disabled: false
  - package: oci://ghcr.io/redhat-developer/rhdh-plugin-export-overlays/red-hat-developer-hub-backstage-plugin-techdocs-mcp-tool:bs_1.45.3__0.3.2
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

#### MCP Server Configuration

Model Context Protocol servers can be extended with custom tools. Configure additional MCP servers in your app-config:

```yaml
# Example MCP server configuration
# (Specific configuration format depends on MCP server implementation)
```

### Features

**AI Chat Interface:**
- Contextual assistance for code development
- Documentation navigation and search
- Workflow guidance and best practices
- Natural language queries about the software catalog

**Software Catalog Integration:**
- Query components, APIs, and resources using natural language
- Get information about ownership, dependencies, and relationships

**TechDocs Integration:**
- Search and navigate technical documentation
- Ask questions about documentation content
- Get contextual help based on current documentation

### Usage

Once enabled, users can:
1. Click the Lightspeed floating button to open the chat interface
2. Ask questions about code, documentation, or workflows
3. Query the software catalog using natural language
4. Navigate TechDocs with AI assistance

### Notes

- Lightspeed is enabled by default for all new RHDH deployments starting from version 1.10
- The flavour includes all necessary UI components and backend services
- MCP tools provide AI access to catalog and documentation data
- Custom MCP servers can be added for organization-specific capabilities

For more information about the Flavour-based configuration system, see the [Configuration documentation](configuration.md#flavours).
