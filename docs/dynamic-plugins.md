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

If plugin dependencies require infrastructural resources (e.g., RoleBindings, CustomResources) and if the User (Administrator) wants it to be deployed (see Note below), they can be specified in the /config/profile/{PROFILE}/plugin-infra directory. For convenience, these can be grouped per plugin in subdirectories.  To create these resources (along with the operator deployment), use the make plugin-infra command. 

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

### Example: Orchestrator plugin dependencies
The orchestrator plugin (as of v1.5.1) consists of three dynamic plugins:
- orchestrator-backend
- orchestrator-frontend
- scaffolder-backend-module
See [example](/examples/orchestrator.yaml) for a complete configuration of the orchestrator plugin.


The orchestrator plugin has the following dependencies:
- A Sonataflowplatform custom resource - created in the namespace of the Backstage CR.
- Knativeeventing and Knativeserving custom resources to be created in the knative-eventing and knative-serving namespaces respectively.
- A set of NetworkPolicies to communicate to Knative resources created in the namespace of Backstage CR.
See [profile/rhdh/operator/plugin-deps/orchestrator](/config/profile/rhdh/operator/plugin-deps/orchestrator)

The orchestrator-backend plugin uses the service sonataflow-platform-data-index-service, which is created by the SonataFlowPlatform CR. This service is used to communicate with the SonataFlow platform.

**Important:** The sonataflowplatform CR contains dataIndex service that requires PostgreSQL database. 

```yaml
      persistence:
        postgresql:
          secretRef:
            name: backstage-psql-secret-{{backstage-name}}
            userKey: POSTGRES_USER
            passwordKey: POSTGRES_PASSWORD
          serviceRef:
            name: backstage-psql-{{backstage-name}}
            namespace: {{backstage-ns}}
            databaseName: backstage_plugin_orchestrator
```

Current **default** implementation of the orchestrator plugin dependencies uses:
- the PstgreSQL database created bt Backstage for Orchestrator plugin, named **backstage_plugin_orchestrator** 
- the Secret created by Backstage operator for the PstgreSQL with **POSTGRES_USER** and **POSTGRES_PASSWORD** keys as the database credentials in the Backstage CR namespace. 
- the Service created by Backstage operator for the PostgreSQL database with the name **backstage-psql-{{backstage-name}}** in the Backstage CR namespace.

**Note:** This default implementation is not recommended for production. For production environments, you should configure an external database connection instead (**TODO: Provide an example with an external database**).

**Known issue:** 
Since DB secret with credentials is automatically generated by the Operator every time new Backstage CR created and sonataflowplatform is not recreated and so uses the "old" secret, the orchestrator plugin will not work properly. To resolve this, the user must manually recreate the SonataFlowPlatform CR or its pods.

The SonataFlowPlatform CR requires the SonataFlow operator to be installed. In an OpenShift environment, it is done by installing the OpenShift Serverless Operator and the OpenShift Serverless Logic Operator.
Additionally, to enable the Backstage operator to work with the SonataFlow platform, its ServiceAccount must be granted the appropriate permissions. 

These manifests are defined as plugin infrastructure in the profile/rhdh/operator/plugin-infra/orchestrator directory.
**Note:** Using plugin infrastructure (plugin-infra) can be risky in production, as it modifies cluster-scoped resources.