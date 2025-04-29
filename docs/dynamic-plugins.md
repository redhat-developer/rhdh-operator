## Dynamic plugins dependency management

### Overview
Dynamic plugins configured for the Backstage CR may require certain Kubernetes resources to be configured to make the plugin work. These are referred to as 'plugin dependencies'. Starting from version 1.7, it is possible to automatically create these resources when the Backstage CR is applied to the cluster.

### Profile Configuration
Plugin dependency configuration for a specific profile is done via the `/config/profile/{PROFILE}/operator/plugin-deps` directory. To enable this, the administrator should:
- Create a directory with the name associated with the plugin (for convenience) and place the required resources as Kubernetes manifests in YAML format within it.
- Create/modify kustomization.yaml to generate a ConfigMap with these files.
- Create/modify a patch which make Kustomize mount this ConfigMap to the Backstage container.

**Example Directory Structure**:
```txt
config/
  profile/
    rhdh/
      operator/
        kustomization.yaml
        example-plugin-patch.yaml
        plugin-deps/
          example/
            dep1.yaml
            dep2.yaml
```
Here, **dep1.yaml** and **dep2.yaml** are the plugin dependencies for the example plugin.

**Notes:**  

* If a resource manifest does not specify a namespace, it will be created in the namespace of the Backstage CR.
* Resources may contain {{backstage-name}} and {{backstage-ns}} placeholders, which will be replaced with the name and namespace of the Backstage CR, respectively.

The `kustomization.yaml` file should contain the following lines:
```yaml
patches:
  - path: example-plugin-patch.yaml
    target:
      kind: Deployment
      name: operator
      
configMapGenerator:
  - files:
      - plugin-deps/example/dep1.yaml
      - plugin-deps/example/dep2.yaml
    name: plugin-deps-example
```
The names of patch file and configmap are arbitrary.

The patch file (`example-plugin-patch.yaml` in this example) should contain the following lines:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: operator
spec:
  selector:
    matchLabels:
      app: backstage-operator
  template:
      spec:
        containers:
          - name: manager
            volumeMounts:
              - name: plugin-deps-example
                mountPath: plugin-deps/example
        volumes:
          - name: plugin-deps-example
            configMap:
              name: plugin-deps-example
```
Ensure that:  
* **spec.template.spec.volumes.configMap.name** matches configMapGenerator.name in the kustomization.yaml file.
* **spec.template.spec.containers.volumeMounts.mountPath** matches the directory path in plugin-deps/example.


### Plugin dependencies infrastructure

If plugin dependencies require infrastructural resources (e.g., RoleBindings, CustomResources), they can be specified in the /config/profile/{PROFILE}/plugin-infra directory. For convenience, these can be grouped per plugin in subdirectories.  To create these resources (along with the operator deployment), use the make plugin-infra command. 

**Note**: Be cautious when running this command on a production cluster, as it may reconfigure cluster-scoped resources.

### Plugin configuration

To create the plugin dependencies when the Backstage CR is applied, they must be referenced in the **dependencies** field of the plugin configuration. The operator will look for the plugin-deps directory in the default-config directory of the profile and create the resources described in the files within this directory.  

Plugin dependencies can be referenced in the dynamic-plugins' ConfigMap. This can either be part of the profile's [default configuration](configuration.md/#default-configuration-files) for all Backstage CRs or part of the [ConfigMap referenced in the Backstage CR](configuration.md/#dynamic-plugins). Starting from version 1.7, plugin dependencies can be included in the dynamic plugin configuration as follows:

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
          - ref: example
```
* In this example, the example dependency is referenced. 
* The operator will look for the plugin-deps/example directory in the profile and create the resources described in the files within this directory.  

This ensures that all required resources for the plugin are automatically created when the Backstage CR is applied.
