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
      - disabled: false
        package: "path-or-url-to-example-plugin"
        dependencies:
          - ref: example-dep
```

In this example, both example-dep1.yaml and example-dep1.yaml will be picked and operator create the resources described in the files. 

See also [Orchestrator plugin dependencies](orchestrator.md#plugin-dependencies) as an example.
