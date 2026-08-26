## Dynamic Plugins registry configuration

Dynamic plugins can be configured to be loaded from different registries, such as NPM or container registries.

### NPM registry

For dynamic plugins packaged in an NPM registry, ensure the **.npmrc** file is properly configured. By default, RHDH uses https://registry.npmjs.org registry and supports additional user-defined **.npmrc** files via the **NPM_CONFIG_USERCONFIG** environment variable, pointing to **/opt/app-root/src/.npmrc.dynamic-plugins/.npmrc** .

The default RHDH configuration includes extra .npmrc settings in **secret-files.yaml**:
```
@redhat:registry=https://npm.registry.redhat.com
```

To use your own **.npmrc** configuration:

* Create a Secret with a .npmrc key containing the content of your .npmrc file.
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-npmrc-secret
type: Opaque
stringData:
  .npmrc: |
    @my:registry=https://npm.my-registry.com
```
* Mount the Secret to the install-dynamic-plugin container by adding the following to the Backstage CR:

```yaml 
apiVersion: rhdh.redhat.com/v1alpha5
#...
spec:
  application:
    extraFiles:
      secrets:
        - name: my-npmrc-secret
          mountPath: /opt/app-root/src/.npmrc.dynamic-plugins
          containers:
            - install-dynamic-plugins          
```

### Container registry

TODO: Dynamic plugins can be configured to use container registries for authentication and image pulling. This section should cover the configuration options available for container registry integration with dynamic plugins.

## Plugin Catalog Configuration

The operator supports loading default plugin configurations from OCI container images (plugin catalogs). For general information about how the catalog index works, see [Using a Catalog Index Image for Default Plugin Configurations](https://github.com/redhat-developer/rhdh/blob/main/docs/dynamic-plugins/installing-plugins.md#using-a-catalog-index-image-for-default-plugin-configurations).

### DevHubPluginCatalog Resources

**Available from operator version 2.0**

Plugin catalogs are defined using `DevHubPluginCatalog` resources in the operator namespace. The operator fetches catalogs from OCI registries, merges them, and makes the configuration available to Backstage instances.

**Default Catalog**: The operator automatically includes the default RHDH plugin catalog (`quay.io/rhdh/plugin-catalog-index:<version>`).

#### Basic Example

```yaml
apiVersion: rhdh.redhat.com/v1alpha5
kind: DevHubPluginCatalog
metadata:
  name: my-catalog
  namespace: rhdh-operator  # Must be in operator namespace
spec:
  source:
    ref: oci://quay.io/my-org/plugin-catalog:v1.0
```

#### Adding Multiple Catalogs

Create additional `DevHubPluginCatalog` resources to add more plugin sources. The operator merges all catalogs automatically.

**Important**: Plugin names must be unique across all catalogs. Duplicate plugin names will cause a reconciliation error.

#### Private Registry Authentication

For private registries, create a Secret with registry credentials in the operator namespace:

```bash
kubectl create secret docker-registry private-registry-creds \
  --namespace=rhdh-operator \
  --docker-server=registry.example.com \
  --docker-username=YOUR_USERNAME \
  --docker-password=YOUR_PASSWORD
```

Then reference it in the DevHubPluginCatalog:

```yaml
apiVersion: rhdh.redhat.com/v1alpha5
kind: DevHubPluginCatalog
metadata:
  name: private-catalog
  namespace: rhdh-operator
spec:
  source:
    ref: oci://registry.example.com/rhdh/plugin-catalog:v1.0
    pullSecret:
      name: private-registry-creds
```

The Secret must be of type `kubernetes.io/dockerconfigjson` and exist in the operator namespace.

#### Custom CA Certificate

For registries using self-signed certificates or internal CAs, create a ConfigMap with the CA certificate in the operator namespace:

```bash
kubectl create configmap internal-ca \
  --namespace=rhdh-operator \
  --from-file=ca.crt=/path/to/ca-certificate.pem
```

Then reference it in the DevHubPluginCatalog:

```yaml
apiVersion: rhdh.redhat.com/v1alpha5
kind: DevHubPluginCatalog
metadata:
  name: internal-catalog
  namespace: rhdh-operator
spec:
  source:
    ref: oci://internal-registry.corp.example.com/rhdh/plugin-catalog:latest
    certificateAuthority:
      name: internal-ca
      key: ca.crt          # Optional, defaults to "ca.crt"
```

#### Skip TLS Verification (Development Only)

```yaml
apiVersion: rhdh.redhat.com/v1alpha5
kind: DevHubPluginCatalog
metadata:
  name: dev-catalog
  namespace: rhdh-operator
spec:
  source:
    ref: oci://dev-registry.local:5000/rhdh/plugin-catalog:dev
    skipTLSVerify: true
```

#### Proxy Settings

The operator respects standard proxy environment variables when fetching catalogs:

- `HTTP_PROXY` / `http_proxy` - proxy for HTTP requests
- `HTTPS_PROXY` / `https_proxy` - proxy for HTTPS requests
- `NO_PROXY` / `no_proxy` - comma-separated list of hosts to bypass proxy

These variables should be set on the operator deployment. For environments with HTTPS-inspecting proxies that use a corporate CA, combine proxy settings with `certificateAuthority` configuration.

#### Manual Refresh

To manually trigger a catalog refresh (e.g., after pushing a new image with the same tag):

```bash
kubectl annotate devhubplugincatalog my-catalog rhdh.redhat.com/refresh=$(date +%s) --overwrite
```

### Extensions Catalog Entities

Starting from version 1.9, the `rhdh` profile of the operator extracts catalog entities from the catalog index image to a new `/extensions` volume mount by default.
This allows the extensions backend providers to automatically discover plugin metadata for display in the RHDH Extensions UI.

More details in [Catalog Entities Extraction](https://github.com/redhat-developer/rhdh/blob/main/docs/dynamic-plugins/installing-plugins.md#catalog-entities-extraction).

### Init-container processing Mode

When `OPERATOR_DP_PROCESSING=false`, the RHDH `install-dynamic-plugins` init container handles catalog fetching using environment variables.

By default, the `rhdh` profile [injects](../config/profile/rhdh/patches/deployment-patch.yaml#L31-L32) the `CATALOG_INDEX_IMAGE` environment variable. To use a different catalog index image, use the `extraEnvs` field in your Backstage CR. See [examples/catalog-index.yaml](../examples/catalog-index.yaml) for an example.

For multiple catalog sources in this mode, use the `EXTRA_CATALOG_INDEX_IMAGES` environment variable. See [Using extra catalog index images](https://github.com/redhat-developer/rhdh/blob/main/docs/dynamic-plugins/installing-plugins.md#using-extra-catalog-index-images) for details.

## Supported Package URL Formats

| Format | Type | Description |
|--------|------|-------------|
| `ref://plugin-name` | Catalog reference | Look up plugin by name, returns full package URL |
| `oci://...{{inherit}}` | Catalog reference | Look up plugin by name, returns full package URL |
| `oci://...` | Direct link | OCI image reference (no resolution) |
| `https://...` | Direct link | HTTPS URL to plugin archive |
| `http://...` | Direct link | HTTP URL to plugin archive |
| `./path` | Direct link | Local filesystem path |

## Plugin URL References

The operator optionally supports special URL reference syntax in plugin package URLs, allowing users to reference plugins from the default configuration by name.

**Operator behavior:**
- The operator resolves all references during ConfigMap merge (before passing to the init container)
- If a reference cannot be resolved, the operator returns an error and the Backstage CR will not reconcile
- Both reference types use **name-based matching** - only the plugin name matters for lookup

### Ref Reference (`ref://`)

Look up a plugin by name and use its full package URL from the default configuration.

```yaml
plugins:
  - package: "ref://backstage-plugin-catalog"
    pluginConfig:
      # your config overrides
```

### Inherit Reference (`:{{inherit}}`)

Look up a plugin by name and use its full package URL from the default configuration. The registry/path in your URL is ignored - only the plugin name matters for matching.

```yaml
plugins:
  # These all match the same base plugin (backstage-plugin-catalog):
  - package: "oci://quay.io/rhdh/backstage-plugin-catalog:{{inherit}}"
  - package: "oci://any-registry/path/backstage-plugin-catalog:{{inherit}}"
```

**Since v2.0.0:** Both `ref://` and `:{{inherit}}` use name-based matching (plugin name only, registry/path ignored). This behavior is slightly different from what is described in [OCI Package Version Inheritance](https://github.com/redhat-developer/rhdh/blob/main/docs/dynamic-plugins/installing-plugins.md#oci-package-version-inheritance) which documents the RHDH init-container behavior (full URL matching).

## Dynamic plugins dependency management

### Overview
Dynamic plugins configured for the Backstage CR may require certain Kubernetes resources to be configured to make the plugin work. These are referred to as 'plugin dependencies'. Starting from version 1.7, it is possible to automatically create these resources when the Backstage CR is applied to the cluster.

### Profile Configuration
Plugin dependency configuration for a specific profile is done via the `/config/profile/{PROFILE}/plugin-deps` directory. To enable this, the administrator should place the required resources as Kubernetes manifests in YAML format within **plugin-deps** directory.

**Example Directory Structure**:
```txt
config/
  profile/
    rhdh/
     kustomization.yaml
     plugin-deps/
        example-dep1.yaml
        example-dep2.yaml
```
Here, **example-dep1.yaml** and **example-dep2.yaml** are the plugin dependencies for the example plugin.

**Notes:**  

* If a resource manifest does not specify a namespace, it will be created in the namespace of the Backstage CR.
* Resources may contain **{{backstage-name}}** and **{{backstage-ns}}** placeholders, which will be replaced with the name and namespace of the Backstage CR, respectively.

The `kustomization.yaml` file should contain the following lines:
```yaml

configMapGenerator:
  - files:
      - plugin-deps/example-dep1.yaml
      - plugin-deps/example-dep2.yaml
    name: plugin-deps
```

### Plugin dependencies infrastructure

If plugin dependencies require infrastructural resources (e.g. other Operators and CRs to be installed) and if the User (Administrator) wants it to be deployed (see Note below), they can be specified in the /config/profile/{PROFILE}/plugin-infra directory. To create these resources (along with the operator deployment), use the `make plugin-infra` command. 

**Note**: Be cautious when running this command on a production cluster, as it may reconfigure cluster-scoped resources.

### Plugin configuration

To create the plugin dependencies when the Backstage CR is applied, they must be referenced in the **dependencies** field of the plugin configuration. The operator will look for the **plugin-deps** directory and create the resources described in the files within this directory.  

Plugin dependencies can be referenced in the dynamic-plugins' ConfigMap. This can either be part of the profile's [default configuration](configuration.md/#default-configuration-files) for all Backstage CRs or part of the [ConfigMap referenced in the Backstage CR](configuration.md/#dynamic-plugins). Starting from version 1.7, plugin dependencies can be included in the dynamic plugin configuration. Each `dependencies.ref` value can either match the full file name or serve as a prefix for the file name. The operator will look for files in the `plugin-deps` directory whose names either start with the specified `ref` value or exactly match it. These files will be used to create the resources described within them. 

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: default-dynamic-plugins
data:
  dynamic-plugins.yaml: |
    includes:
      - dynamic-plugins.default.yaml
    plugins:
      - enabled: true
        package: "path-or-url-to-example-plugin"
        dependencies:
          - ref: example-dep
```

In this example, both example-dep1.yaml and example-dep1.yaml will be picked and operator create the resources described in the files. 

Same as other plugin configuration options, the dependencies can be defined in the default configuration for the profile or in the ConfigMap referenced in the Backstage CR. If a dependency is defined in both places, the operator will replace the one defined in the default configuration with the one defined in the Backstage CR.
So if you want to define dependencies in CR, you need to redefine all of them in the CR, even if some of them are already defined in the default configuration. In a case if you want to clean up the dependencies defined in the default configuration, you can set `dependencies: []` in the CR.

See also [Orchestrator plugin dependencies](orchestrator.md#plugin-dependencies) as an example.
